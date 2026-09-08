package promptlearning

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type RedactionKind string

const (
	RedactionURLCredential RedactionKind = "url_credential"
	RedactionJWT           RedactionKind = "jwt"
	RedactionAPIKey        RedactionKind = "api_key"
	RedactionEmail         RedactionKind = "email"
	RedactionPhone         RedactionKind = "phone"
	RedactionSecret        RedactionKind = "secret"
	RedactionHighEntropy   RedactionKind = "high_entropy"
)

type RedactionSummary map[RedactionKind]int

var (
	urlCredentialPattern = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://)([^/@\s]+)@`)
	jwtPattern           = regexp.MustCompile(`\b[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`)
	apiKeyPattern        = regexp.MustCompile(`(?i)\b(?:sk-[a-z0-9_-]{16,}|gh[pousr]_[a-z0-9]{20,}|xox[baprs]-[a-z0-9-]{10,}|AKIA[0-9A-Z]{16})\b`)
	emailPattern         = regexp.MustCompile(`(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b`)
	phonePattern         = regexp.MustCompile(`(?:\+?\d[\d ()-]{7,}\d)`)
	highEntropyPattern   = regexp.MustCompile(`[A-Za-z0-9+/=_-]{24,}`)
)

var contextSecretLabels = []string{
	"authorization",
	"refresh_token",
	"refresh-token",
	"refresh token",
	"access_token",
	"access-token",
	"access token",
	"api_key",
	"api-key",
	"api key",
	"password",
	"passwd",
	"secret",
	"token",
	"pwd",
}

const internalMarkerPrefix = "\x00R"

type redactionState struct {
	summary      RedactionSummary
	replacements []string
}

func newRedactionState() *redactionState {
	return &redactionState{summary: make(RedactionSummary)}
}

func (state *redactionState) marker(kind RedactionKind, publicMarker string) string {
	state.summary[kind]++
	index := len(state.replacements)
	state.replacements = append(state.replacements, publicMarker)
	return internalMarkerPrefix + strconv.Itoa(index) + "\x00"
}

type redactionRestoreStatus uint8

const (
	redactionRestoreOK redactionRestoreStatus = iota
	redactionRestoreTooLarge
	redactionRestoreInvalidMarker
)

func (state *redactionState) markerIndex(value string) (int, bool) {
	if !strings.HasPrefix(value, internalMarkerPrefix) || !strings.HasSuffix(value, "\x00") {
		return 0, false
	}
	digits := value[len(internalMarkerPrefix) : len(value)-1]
	if digits == "" {
		return 0, false
	}
	index, err := strconv.Atoi(digits)
	if err != nil || index < 0 || strconv.Itoa(index) != digits {
		return 0, false
	}
	return index, index < len(state.replacements)
}

func (state *redactionState) restore(text string, maxBytes int) (string, redactionRestoreStatus) {
	var output strings.Builder
	if len(text) < maxBytes {
		output.Grow(len(text))
	} else {
		output.Grow(maxBytes)
	}

	for cursor := 0; cursor < len(text); {
		markerOffset := strings.IndexByte(text[cursor:], '\x00')
		if markerOffset < 0 {
			if !appendWithinLimit(&output, text[cursor:], maxBytes) {
				return "", redactionRestoreTooLarge
			}
			break
		}
		markerStart := cursor + markerOffset
		if !appendWithinLimit(&output, text[cursor:markerStart], maxBytes) {
			return "", redactionRestoreTooLarge
		}
		markerEndOffset := strings.IndexByte(text[markerStart+1:], '\x00')
		if markerEndOffset < 0 {
			return "", redactionRestoreInvalidMarker
		}
		markerEnd := markerStart + 1 + markerEndOffset + 1
		index, ok := state.markerIndex(text[markerStart:markerEnd])
		if !ok {
			return "", redactionRestoreInvalidMarker
		}
		if !appendWithinLimit(&output, state.replacements[index], maxBytes) {
			return "", redactionRestoreTooLarge
		}
		cursor = markerEnd
	}
	return output.String(), redactionRestoreOK
}

func appendWithinLimit(output *strings.Builder, value string, maxBytes int) bool {
	if len(value) > maxBytes-output.Len() {
		return false
	}
	output.WriteString(value)
	return true
}

func redactText(text string, maxBytes int) (string, RedactionSummary, redactionRestoreStatus) {
	state := newRedactionState()
	text = redactURLCredentials(text, state)
	text = replaceMatches(text, jwtPattern, RedactionJWT, state, "[REDACTED:JWT]")
	text = replaceMatches(text, apiKeyPattern, RedactionAPIKey, state, "[REDACTED:API_KEY]")
	text = replaceMatches(text, emailPattern, RedactionEmail, state, "[REDACTED:EMAIL]")
	text = replaceMatches(text, phonePattern, RedactionPhone, state, "[REDACTED:PHONE]")

	// Layer two catches context-labelled values and otherwise unlabelled
	// high-entropy tokens that the structured patterns above do not recognize.
	text = redactContextSecrets(text, state)
	text = redactHighEntropy(text, state)
	text, status := state.restore(text, maxBytes)
	return text, state.summary, status
}

func redactURLCredentials(text string, state *redactionState) string {
	return urlCredentialPattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := urlCredentialPattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return state.marker(RedactionURLCredential, "[REDACTED:URL_CREDENTIAL]")
		}
		return parts[1] + state.marker(RedactionURLCredential, "[REDACTED:URL_CREDENTIAL]") + "@"
	})
}

func replaceMatches(text string, pattern *regexp.Regexp, kind RedactionKind, state *redactionState, publicMarker string) string {
	return pattern.ReplaceAllStringFunc(text, func(string) string {
		return state.marker(kind, publicMarker)
	})
}

func redactContextSecrets(text string, state *redactionState) string {
	var output strings.Builder
	output.Grow(len(text))
	writtenThrough := 0
	position := contextRecordPosition{inIndent: true}

	for cursor := 0; cursor < len(text); {
		label, labelEnd, ok := contextSecretLabelAt(text, cursor)
		if !ok {
			_, size := utf8.DecodeRuneInString(text[cursor:])
			next := cursor + size
			position.advance(text, cursor, next)
			cursor = next
			continue
		}
		labelIndent := position.indent
		valueStart, startStatus := contextValueStart(text, cursor, labelEnd, labelIndent)
		if startStatus == contextValueAbsent {
			position.advance(text, cursor, labelEnd)
			cursor = labelEnd
			continue
		}
		if startStatus == contextValueAmbiguous {
			replacementStart := contextLabelStart(text, cursor, labelEnd)
			output.WriteString(text[writtenThrough:replacementStart])
			output.WriteString(state.marker(RedactionSecret, "[REDACTED:SECRET]"))
			writtenThrough = len(text)
			cursor = len(text)
			continue
		}

		credentialStart := valueStart
		if label == "authorization" {
			if bearerEnd, bearer := bearerCredentialStart(text, valueStart); bearer {
				credentialStart = bearerEnd
			}
		}
		valueEnd, failClosed := contextValueEnd(text, credentialStart, labelIndent)
		replacementStart, replacementEnd := contextReplacementSpan(text, credentialStart, valueEnd)
		if failClosed {
			replacementStart = skipContextHorizontalSpace(text, credentialStart)
			replacementEnd = len(text)
		}
		if replacementEnd == replacementStart {
			position.advance(text, cursor, valueEnd)
			cursor = valueEnd
			continue
		}

		// Only an entire, valid marker created by this state retains its precise
		// type. Public marker text, malformed markers, and valid markers with any
		// prefix or suffix cause the whole context value to be redacted again.
		if _, exactMarker := state.markerIndex(text[replacementStart:replacementEnd]); exactMarker {
			position.advance(text, cursor, valueEnd)
			cursor = valueEnd
			continue
		}
		output.WriteString(text[writtenThrough:replacementStart])
		output.WriteString(state.marker(RedactionSecret, "[REDACTED:SECRET]"))
		writtenThrough = replacementEnd
		position.advance(text, cursor, valueEnd)
		cursor = valueEnd
	}
	output.WriteString(text[writtenThrough:])
	return output.String()
}

func contextSecretLabelAt(text string, start int) (string, int, bool) {
	if contextLabelWordBefore(text, start) {
		return "", start, false
	}
	for _, label := range contextSecretLabels {
		end := start + len(label)
		if end > len(text) || !strings.EqualFold(text[start:end], label) {
			continue
		}
		if contextLabelWordAt(text, end) {
			continue
		}
		return label, end, true
	}
	return "", start, false
}

// Context labels are ASCII identifiers, but their boundaries are Unicode word
// boundaries. Treating only ASCII letters as words turns an ASCII suffix in
// ordinary multilingual prose into a credential label and can redact the rest
// of that record. The kernel rejects invalid UTF-8 before this scanner runs;
// RuneError remains a non-word fallback for defensive direct callers.
func contextLabelWordBefore(text string, position int) bool {
	if position == 0 {
		return false
	}
	char, _ := utf8.DecodeLastRuneInString(text[:position])
	return isContextLabelWord(char)
}

func contextLabelWordAt(text string, position int) bool {
	if position >= len(text) {
		return false
	}
	char, _ := utf8.DecodeRuneInString(text[position:])
	return isContextLabelWord(char)
}

func isContextLabelWord(char rune) bool {
	return char != utf8.RuneError && (unicode.IsLetter(char) || unicode.IsNumber(char) ||
		unicode.IsMark(char) || unicode.Is(unicode.Pc, char))
}

type contextValueStartStatus uint8

const (
	contextValueAbsent contextValueStartStatus = iota
	contextValueReady
	contextValueAmbiguous
)

type contextRecordPosition struct {
	indent   int
	inIndent bool
}

func (position *contextRecordPosition) advance(text string, start, end int) {
	for cursor := start; cursor < end; {
		char, size := utf8.DecodeRuneInString(text[cursor:end])
		cursor += size
		if contextRecordBoundary(char) {
			position.indent = 0
			position.inIndent = true
			continue
		}
		if !position.inIndent {
			continue
		}
		if unicode.IsSpace(char) {
			position.indent++
			continue
		}
		position.inIndent = false
	}
}

func contextQuotedLabel(text string, labelStart, labelEnd int) (byte, bool) {
	if labelStart == 0 || labelEnd >= len(text) {
		return 0, false
	}
	quote := text[labelStart-1]
	return quote, (quote == '"' || quote == '\'') && text[labelEnd] == quote
}

func contextLabelStart(text string, labelStart, labelEnd int) int {
	if _, quoted := contextQuotedLabel(text, labelStart, labelEnd); quoted {
		return labelStart - 1
	}
	return labelStart
}

func contextValueStart(text string, labelStart, labelEnd, labelIndent int) (int, contextValueStartStatus) {
	cursor := labelEnd
	_, quotedLabel := contextQuotedLabel(text, labelStart, labelEnd)
	if quotedLabel {
		cursor++
	}
	spaceStart := cursor
	cursor = skipContextHorizontalSpace(text, cursor)
	hadSpace := cursor > spaceStart
	if cursor == len(text) {
		return cursor, contextValueAbsent
	}
	if contextRecordBoundaryAt(text, cursor) {
		// A separator appearing on a later record (notably pretty JSON) is
		// structurally ambiguous. Preserve no suffix that might contain its value.
		return cursor, contextValueAmbiguous
	}
	if cursor < len(text) && (text[cursor] == ':' || text[cursor] == '=') {
		cursor++
		cursor = skipContextHorizontalSpace(text, cursor)
		if cursor == len(text) {
			return cursor, contextValueAbsent
		}
		if contextRecordBoundaryAt(text, cursor) {
			// A quoted key followed by a record break is the pretty-JSON case.
			// For an unquoted YAML key, an equally or less indented next record
			// proves the value is empty and preserves the next sibling field.
			if quotedLabel || contextRecordContinuesValue(text, cursor, labelIndent) {
				return cursor, contextValueAmbiguous
			}
			return cursor, contextValueAbsent
		}
	} else if !hadSpace {
		return labelEnd, contextValueAbsent
	}
	return cursor, contextValueReady
}

func skipContextHorizontalSpace(text string, start int) int {
	cursor := start
	for cursor < len(text) {
		char, size := utf8.DecodeRuneInString(text[cursor:])
		// Record separators are boundaries outside a quoted value. Never skip
		// them here: doing so could turn an empty YAML value into the next field.
		if contextRecordBoundary(char) || !unicode.IsSpace(char) {
			break
		}
		cursor += size
	}
	return cursor
}

func contextRecordBoundary(char rune) bool {
	return char == '\n' || char == '\u2028' || char == '\u2029'
}

func contextRecordBoundaryAt(text string, start int) bool {
	if start >= len(text) {
		return false
	}
	char, _ := utf8.DecodeRuneInString(text[start:])
	return contextRecordBoundary(char)
}

func contextRecordContinuesValue(text string, boundaryStart, labelIndent int) bool {
	cursor := boundaryStart
	for cursor < len(text) {
		boundary, size := utf8.DecodeRuneInString(text[cursor:])
		if !contextRecordBoundary(boundary) {
			return true
		}
		cursor += size
		lineStart := cursor
		cursor = skipContextHorizontalSpace(text, cursor)
		if cursor == len(text) {
			return false
		}
		if contextRecordBoundaryAt(text, cursor) {
			continue
		}
		indent := utf8.RuneCountInString(text[lineStart:cursor])
		if indent > labelIndent {
			return true
		}
		if indent != labelIndent || text[cursor] != '-' {
			return false
		}
		sequenceEnd := cursor + 1
		if sequenceEnd == len(text) || contextRecordBoundaryAt(text, sequenceEnd) {
			return true
		}
		next, _ := utf8.DecodeRuneInString(text[sequenceEnd:])
		return unicode.IsSpace(next) && !contextRecordBoundary(next)
	}
	return false
}

func bearerCredentialStart(text string, valueStart int) (int, bool) {
	const bearer = "bearer"
	bearerEnd := valueStart + len(bearer)
	if bearerEnd > len(text) || !strings.EqualFold(text[valueStart:bearerEnd], bearer) {
		return valueStart, false
	}
	if contextLabelWordAt(text, bearerEnd) {
		return valueStart, false
	}
	credentialStart := skipContextHorizontalSpace(text, bearerEnd)
	if credentialStart == bearerEnd || credentialStart == len(text) {
		return valueStart, false
	}
	return credentialStart, true
}

func contextValueEnd(text string, start, labelIndent int) (int, bool) {
	start = skipContextHorizontalSpace(text, start)
	if start == len(text) {
		return start, false
	}
	first, firstSize := utf8.DecodeRuneInString(text[start:])
	if first == '[' || first == '{' {
		// Composite ownership is deliberately not inferred here. A malformed
		// close, nested delimiter, or public marker could otherwise manufacture
		// an early boundary, so the safe result consumes the remainder.
		return len(text), true
	}
	if first == '|' || first == '>' {
		return len(text), true
	}
	if first == '"' || first == '\'' {
		escaped := false
		for cursor := start + firstSize; cursor < len(text); {
			char, size := utf8.DecodeRuneInString(text[cursor:])
			if escaped {
				escaped = false
				cursor += size
				continue
			}
			if char == '\\' {
				escaped = true
				cursor += size
				continue
			}
			if char != first {
				cursor += size
				continue
			}
			afterQuote := skipContextHorizontalSpace(text, cursor+size)
			if afterQuote == len(text) {
				return afterQuote, false
			}
			next, _ := utf8.DecodeRuneInString(text[afterQuote:])
			if contextRecordBoundary(next) {
				if contextRecordContinuesValue(text, afterQuote, labelIndent) {
					return len(text), true
				}
				return afterQuote, false
			}
			if next == ',' || next == ';' || next == '}' {
				return afterQuote, false
			}
			return len(text), true
		}
		// An unterminated quote consumes the remainder as the sensitive value.
		return len(text), false
	}

	var quote rune
	escaped := false
	for cursor := start; cursor < len(text); {
		char, size := utf8.DecodeRuneInString(text[cursor:])
		if quote != 0 {
			if escaped {
				escaped = false
				cursor += size
				continue
			}
			if char == '\\' {
				escaped = true
				cursor += size
				continue
			}
			if char == quote {
				quote = 0
			}
			cursor += size
			continue
		}
		if char == '"' || char == '\'' {
			quote = char
			cursor += size
			continue
		}
		// Any opener makes delimiter ownership ambiguous. Public redaction
		// markers also begin with '[', so this deliberately consumes their suffix.
		if char == '[' || char == '{' {
			return len(text), true
		}
		if char == ',' || char == ';' {
			afterDelimiter := skipContextHorizontalSpace(text, cursor+size)
			if contextRecordBoundaryAt(text, afterDelimiter) &&
				contextRecordContinuesValue(text, afterDelimiter, labelIndent) {
				return len(text), true
			}
			return cursor, false
		}
		if char == '}' {
			return cursor, false
		}
		if contextRecordBoundary(char) {
			if contextRecordContinuesValue(text, cursor, labelIndent) {
				return len(text), true
			}
			return cursor, false
		}
		cursor += size
	}
	return len(text), false
}

func contextReplacementSpan(text string, start, end int) (int, int) {
	start = skipContextHorizontalSpace(text, start)
	end = trimContextSpace(text, start, end)
	if end-start >= 2 && (text[start] == '"' || text[start] == '\'') && text[end-1] == text[start] {
		return start + 1, end - 1
	}
	return start, end
}

func trimContextSpace(text string, start, end int) int {
	for end > start {
		char, size := utf8.DecodeLastRuneInString(text[start:end])
		if !unicode.IsSpace(char) {
			break
		}
		end -= size
	}
	return end
}

func redactHighEntropy(text string, state *redactionState) string {
	return highEntropyPattern.ReplaceAllStringFunc(text, func(token string) string {
		if !isHighEntropyToken(token) {
			return token
		}
		return state.marker(RedactionHighEntropy, "[REDACTED:HIGH_ENTROPY]")
	})
}

func isHighEntropyToken(token string) bool {
	var lower, upper, digit, symbol bool
	counts := make(map[byte]int)
	for index := 0; index < len(token); index++ {
		char := token[index]
		counts[char]++
		switch {
		case char >= 'a' && char <= 'z':
			lower = true
		case char >= 'A' && char <= 'Z':
			upper = true
		case char >= '0' && char <= '9':
			digit = true
		default:
			symbol = true
		}
	}
	categories := 0
	for _, present := range []bool{lower, upper, digit, symbol} {
		if present {
			categories++
		}
	}
	if categories < 2 {
		return false
	}

	entropy := 0.0
	length := float64(len(token))
	for _, count := range counts {
		probability := float64(count) / length
		entropy -= probability * math.Log2(probability)
	}
	return entropy >= 3.5
}
