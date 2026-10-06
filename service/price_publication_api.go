package service

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/ForceMind/MyAPI/model"
)

var ErrPricePublicationInput = errors.New("invalid or unqualified price publication input")

type PricePublicationPreviewRow struct {
	Model             string                           `json:"model"`
	CurrentMode       string                           `json:"current_mode"`
	CurrentExpression string                           `json:"current_expression"`
	Locked            bool                             `json:"locked"`
	Candidate         *OpenAIPricePublicationCandidate `json:"candidate"`
	Eligible          bool                             `json:"eligible"`
}

type PricePublicationPreview struct {
	SourceSHA256   string                       `json:"source_sha256"`
	ExpectedDigest string                       `json:"expected_digest"`
	Revision       int64                        `json:"revision"`
	Rows           []PricePublicationPreviewRow `json:"rows"`
}

func PreviewOpenAIPricePublication(ctx context.Context, digest string) (*PricePublicationPreview, error) {
	source, err := LoadFrozenOpenAIPriceSource(ctx, model.DB, digest)
	if err != nil {
		return nil, err
	}
	current, err := model.ReadPricePublicationSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	expected, err := current.Digest()
	if err != nil {
		return nil, err
	}
	preview := &PricePublicationPreview{SourceSHA256: digest, ExpectedDigest: expected, Revision: current.State.Revision, Rows: make([]PricePublicationPreviewRow, 0, len(source.Models))}
	for _, price := range source.Models {
		candidate, candidateErr := BuildOpenAIPricePublicationCandidate(source, price.Model)
		mode := current.Modes[price.Model]
		if mode == "" {
			mode = "ratio"
		}
		locked := current.State.Models[price.Model].Locked
		preview.Rows = append(preview.Rows, PricePublicationPreviewRow{Model: price.Model, CurrentMode: mode, CurrentExpression: current.Expressions[price.Model], Locked: locked, Candidate: candidate, Eligible: candidateErr == nil && !locked})
	}
	return preview, nil
}

type PricePublicationSelection struct {
	Model  string `json:"model"`
	Locked *bool  `json:"locked"`
}

type PricePublicationRequest struct {
	ID             string                      `json:"id"`
	ExpectedDigest string                      `json:"expected_digest"`
	Action         string                      `json:"action"`
	SourceSHA256   string                      `json:"source_sha256,omitempty"`
	RollbackOf     string                      `json:"rollback_of,omitempty"`
	Models         []PricePublicationSelection `json:"models,omitempty"`
	Confirmed      bool                        `json:"confirmed"`
}

func ApplyOpenAIPricePublication(ctx context.Context, actorID int, request PricePublicationRequest) (*model.PricePublication, error) {
	if !request.Confirmed || len(request.Models) > 64 || actorID <= 0 {
		return nil, ErrPricePublicationInput
	}
	seen := map[string]bool{}
	for _, selection := range request.Models {
		if len(selection.Model) > 191 || !openAIPriceModelPattern.MatchString(selection.Model) || seen[selection.Model] {
			return nil, ErrPricePublicationInput
		}
		seen[selection.Model] = true
	}
	for _, digest := range []string{request.ID, request.ExpectedDigest, request.SourceSHA256, request.RollbackOf} {
		if digest == "" {
			continue
		}
		decoded, err := hex.DecodeString(digest)
		if err != nil || len(decoded) != 32 || hex.EncodeToString(decoded) != digest {
			return nil, ErrPricePublicationInput
		}
	}
	command := model.PricePublicationCommand{ID: request.ID, ExpectedDigest: request.ExpectedDigest, ActorID: actorID, Action: request.Action, RollbackOf: request.RollbackOf}
	switch request.Action {
	case "publish":
		if len(request.Models) == 0 || request.RollbackOf != "" {
			return nil, ErrPricePublicationInput
		}
		source, err := LoadFrozenOpenAIPriceSource(ctx, model.DB, request.SourceSHA256)
		if err != nil {
			return nil, err
		}
		for _, selection := range request.Models {
			if selection.Locked == nil {
				return nil, ErrPricePublicationInput
			}
			candidate, err := BuildOpenAIPricePublicationCandidate(source, selection.Model)
			if err != nil {
				return nil, fmt.Errorf("%w: %v", ErrPricePublicationInput, err)
			}
			command.Changes = append(command.Changes, model.PricePublicationChange{Model: selection.Model, Expression: candidate.Expression, SourceSHA256: candidate.SourceSHA256, Locked: *selection.Locked})
		}
	case "lock":
		if len(request.Models) == 0 || request.SourceSHA256 != "" || request.RollbackOf != "" {
			return nil, ErrPricePublicationInput
		}
		for _, selection := range request.Models {
			if selection.Locked == nil {
				return nil, ErrPricePublicationInput
			}
			command.Changes = append(command.Changes, model.PricePublicationChange{Model: selection.Model, Locked: *selection.Locked})
		}
	case "rollback":
		if len(request.Models) != 0 || request.SourceSHA256 != "" || len(request.RollbackOf) != 64 {
			return nil, ErrPricePublicationInput
		}
	default:
		return nil, ErrPricePublicationInput
	}
	return model.ApplyPricePublication(ctx, command)
}
