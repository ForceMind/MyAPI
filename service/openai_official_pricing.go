package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"github.com/ForceMind/MyAPI/model"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const openAIOfficialPricingURL = "https://developers.openai.com/api/docs/pricing.md"
const maxOpenAIPriceDocumentBytes = model.MaxOfficialPriceDocumentBytes

var openAIPriceAmountPattern = regexp.MustCompile(`^\$(?:0|[1-9][0-9]*)(?:\.[0-9]+)?$`)
var openAIPriceModelPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.:-]*$`)
var openAIOfficialPricingClient = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		IdleConnTimeout:       30 * time.Second,
		MaxIdleConnsPerHost:   2,
		MaxConnsPerHost:       2,
	},
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// Decimal strings preserve the source precision. Null means not quoted by
// the source, not a zero-price capability or a guessed cache multiplier.
type OpenAIOfficialTokenRates struct {
	Input       string  `json:"input_usd_per_million"`
	CachedInput *string `json:"cached_input_usd_per_million"`
	CacheWrite  *string `json:"cache_write_usd_per_million"`
	Output      string  `json:"output_usd_per_million"`
}

type OpenAIOfficialModelPrice struct {
	Model        string                    `json:"model"`
	SourceLabel  string                    `json:"source_label"`
	ShortContext OpenAIOfficialTokenRates  `json:"short_context"`
	LongContext  *OpenAIOfficialTokenRates `json:"long_context"`
}

type OpenAIOfficialPriceSnapshot struct {
	sourceDocument string
	SourceURL      string                     `json:"source_url"`
	FetchedAt      int64                      `json:"fetched_at"`
	ContentSHA256  string                     `json:"content_sha256"`
	Currency       string                     `json:"currency"`
	UnitTokens     int                        `json:"unit_tokens"`
	ServiceTier    string                     `json:"service_tier"`
	Scope          string                     `json:"scope"`
	Models         []OpenAIOfficialModelPrice `json:"models"`
}

func FetchOpenAIOfficialPricing(ctx context.Context) (*OpenAIOfficialPriceSnapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openAIOfficialPricingURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/markdown, text/plain;q=0.9")
	resp, err := openAIOfficialPricingClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("official pricing HTTP status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxOpenAIPriceDocumentBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxOpenAIPriceDocumentBytes {
		return nil, fmt.Errorf("official pricing document too large")
	}
	models, err := parseOpenAIStandardTextPrices(body)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(body)
	return &OpenAIOfficialPriceSnapshot{
		sourceDocument: string(body),
		SourceURL:      openAIOfficialPricingURL, FetchedAt: time.Now().UTC().Unix(),
		ContentSHA256: hex.EncodeToString(digest[:]), Currency: "USD", UnitTokens: 1_000_000,
		ServiceTier: "standard", Scope: "text-token-price-source-not-published", Models: models,
	}, nil
}

func parseOpenAIStandardTextPrices(body []byte) ([]OpenAIOfficialModelPrice, error) {
	expectedHeader := []string{"Model", "Short context input", "Short context cached input", "Short context cache writes", "Short context output", "Long context input", "Long context cached input", "Long context cache writes", "Long context output"}
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 4096), maxOpenAIPriceDocumentBytes)
	section, header, separator, millionTokenUnit := false, false, false, false
	seen := make(map[string]bool)
	var models []OpenAIOfficialModelPrice
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !section {
			if strings.HasPrefix(line, "Prices per ") {
				millionTokenUnit = line == "Prices per 1M tokens."
			}
			if line == "### Standard pricing data" {
				if !millionTokenUnit {
					return nil, fmt.Errorf("unsupported official pricing unit")
				}
				section = true
			}
			continue
		}
		if line == "" {
			if len(models) > 0 {
				break
			}
			continue
		}
		if !strings.HasPrefix(line, "|") {
			if len(models) > 0 {
				break
			}
			return nil, fmt.Errorf("unsupported official standard pricing table")
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		if len(cells) != len(expectedHeader) {
			return nil, fmt.Errorf("unsupported official pricing columns")
		}
		if !header {
			for i := range cells {
				if cells[i] != expectedHeader[i] {
					return nil, fmt.Errorf("unsupported official pricing header")
				}
			}
			header = true
			continue
		}
		if !separator {
			for _, cell := range cells {
				if cell == "" || strings.Trim(cell, "-:") != "" {
					return nil, fmt.Errorf("invalid official pricing separator")
				}
			}
			separator = true
			continue
		}
		name := strings.Fields(cells[0])
		if len(name) == 0 || !openAIPriceModelPattern.MatchString(name[0]) || seen[name[0]] {
			return nil, fmt.Errorf("invalid or duplicate official pricing model")
		}
		short, err := parseOpenAIContextRates(cells[1:5], false)
		if err != nil {
			return nil, err
		}
		long, err := parseOpenAIContextRates(cells[5:9], true)
		if err != nil {
			return nil, err
		}
		seen[name[0]] = true
		models = append(models, OpenAIOfficialModelPrice{Model: name[0], SourceLabel: cells[0], ShortContext: *short, LongContext: long})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("official standard text pricing unavailable")
	}
	return models, nil
}

func parseOpenAIContextRates(cells []string, optional bool) (*OpenAIOfficialTokenRates, error) {
	var amounts [4]*string
	missing := 0
	for i, cell := range cells {
		if cell == "-" {
			missing++
			continue
		}
		if !openAIPriceAmountPattern.MatchString(cell) {
			return nil, fmt.Errorf("invalid official token price")
		}
		value := strings.TrimPrefix(cell, "$")
		amounts[i] = &value
	}
	if optional && missing == 4 {
		return nil, nil
	}
	if amounts[0] == nil || amounts[3] == nil {
		return nil, fmt.Errorf("incomplete official context price")
	}
	return &OpenAIOfficialTokenRates{Input: *amounts[0], CachedInput: amounts[1], CacheWrite: amounts[2], Output: *amounts[3]}, nil
}
