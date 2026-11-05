package restapi

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/app/metadatasettings"
	"github.com/magpeek/mivo/internal/domain"
)

// Operation names of the settings and status endpoints, as in api/openapi.yaml, for logs.
const (
	operationGetMetadataSettings    = "getMetadataSettings"
	operationUpdateMetadataSettings = "updateMetadataSettings"
	operationGetMetadataStatus      = "getMetadataStatus"
)

// Bounds of MetadataSettingsUpdate (api/openapi.yaml).
const (
	maxSettingsLanguageLength = 35
	maxProviderToggles        = 16
	fieldProviders            = "providers"
	fieldTMDBAPIKey           = "tmdbApiKey" //nolint:gosec // a request property name, not a credential
	fieldSettingsLanguage     = "language"
)

// GetMetadataSettings answers the online metadata settings. Keys are never part of them.
func (api *API) GetMetadataSettings(ctx context.Context, _ apigen.GetMetadataSettingsRequestObject) (apigen.GetMetadataSettingsResponseObject, error) {
	if api.metadata.Settings == nil {
		problem := notImplementedProblem()

		return apigen.GetMetadataSettingsdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
	}

	settings, err := api.metadata.Settings.Settings(ctx)
	if err != nil {
		return nil, operationFailure(operationGetMetadataSettings, err)
	}

	return apigen.GetMetadataSettings200JSONResponse(metadataSettingsResponse(settings)), nil
}

// UpdateMetadataSettings applies a change and answers the settings after it. A malformed
// language answers 400 "language_invalid", an unknown provider, a malformed key or an empty
// change 400 "invalid" with the field.
func (api *API) UpdateMetadataSettings(ctx context.Context, request apigen.UpdateMetadataSettingsRequestObject) (apigen.UpdateMetadataSettingsResponseObject, error) {
	if api.metadata.Settings == nil {
		problem := notImplementedProblem()

		return apigen.UpdateMetadataSettingsdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
	}

	change, err := settingsChange(request.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationUpdateMetadataSettings, err)
	}

	settings, err := api.metadata.Settings.Update(ctx, change)

	var fieldErr *metadatasettings.FieldError
	if errors.As(err, &fieldErr) {
		return nil, fmt.Errorf("%s: %w", operationUpdateMetadataSettings, settingsFieldError(fieldErr))
	}

	if err != nil {
		return nil, operationFailure(operationUpdateMetadataSettings, err)
	}

	return apigen.UpdateMetadataSettings200JSONResponse(metadataSettingsResponse(settings)), nil
}

// settingsChange checks the update body against the contract and converts it. The Cover Art
// Archive has no switch of its own: it follows MusicBrainz (ADR 0031 §2), so toggling it
// toggles MusicBrainz.
func settingsChange(body *apigen.UpdateMetadataSettingsJSONRequestBody) (metadatasettings.Change, error) {
	if body == nil || (body.OnlineEnabled == nil && body.Language == nil && body.Providers == nil && body.TmdbAPIKey == nil) {
		return metadatasettings.Change{}, fmt.Errorf("no setting to change: %w", domain.ErrInvalid)
	}

	if body.Language != nil && len(*body.Language) > maxSettingsLanguageLength {
		return metadatasettings.Change{}, &settingsLanguageError{}
	}

	if body.TmdbAPIKey != nil && len(*body.TmdbAPIKey) > metadatasettings.MaxKeyLength {
		return metadatasettings.Change{}, invalidParameter(fieldTMDBAPIKey, "too long")
	}

	change := metadatasettings.Change{Online: body.OnlineEnabled, Language: body.Language, TMDBKey: body.TmdbAPIKey}

	if body.Providers == nil {
		return change, nil
	}

	if err := applyToggles(&change, *body.Providers); err != nil {
		return metadatasettings.Change{}, err
	}

	return change, nil
}

// applyToggles sets the provider switches of change. An unknown provider, or two entries
// that ask for different values of one switch (MusicBrainz and the Cover Art Archive share
// one), answer invalid with the entry as field.
func applyToggles(change *metadatasettings.Change, toggles []apigen.MetadataProviderToggle) error {
	if len(toggles) > maxProviderToggles {
		return invalidParameter(fieldProviders, fmt.Sprintf("more than %d entries", maxProviderToggles))
	}

	for index, toggle := range toggles {
		var target **bool

		switch toggle.Provider {
		case apigen.MetadataProviderTmdb:
			target = &change.TMDB
		case apigen.MetadataProviderMusicbrainz, apigen.MetadataProviderCoverartarchive:
			target = &change.MusicBrainz
		default:
			return invalidParameter(fmt.Sprintf("%s[%d]", fieldProviders, index), "unknown provider")
		}

		if *target != nil && **target != toggle.Enabled {
			return invalidParameter(fmt.Sprintf("%s[%d]", fieldProviders, index), "contradicts an earlier entry for the same switch")
		}

		enabled := toggle.Enabled
		*target = &enabled
	}

	return nil
}

// settingsFieldError turns a rejected setting into its problem: language_invalid for the
// language (as in updateLibrary), invalid with the field otherwise.
func settingsFieldError(err *metadatasettings.FieldError) error {
	if err.Field == metadatasettings.FieldLanguage {
		return &settingsLanguageError{}
	}

	return invalidParameter(string(err.Field), err.Reason)
}

// settingsLanguageError is a malformed metadata language in the settings; problemOf maps it
// to 400 language_invalid with field "language".
type settingsLanguageError struct{}

func (*settingsLanguageError) Error() string { return "metadata language is no language tag" }

func (*settingsLanguageError) Unwrap() error { return domain.ErrInvalid }

// settingsProblem maps a settingsLanguageError; it reports false for every other error.
func settingsProblem(err error) (apigen.Problem, bool) {
	var languageErr *settingsLanguageError
	if !errors.As(err, &languageErr) {
		return apigen.Problem{}, false
	}

	return fieldRequestProblem(CodeLanguageInvalid, fieldSettingsLanguage, "The metadata language is not a valid language tag."), true
}

func metadataSettingsResponse(settings metadatasettings.Settings) apigen.MetadataSettings {
	keyStatus := apigen.ProviderKeyStatus(settings.TMDBKey)

	return apigen.MetadataSettings{
		OnlineEnabled: settings.Online,
		Language:      settings.Language,
		Providers: []apigen.MetadataProviderSetting{
			{Provider: apigen.MetadataProviderTmdb, Enabled: settings.TMDB, KeyStatus: &keyStatus},
			{Provider: apigen.MetadataProviderMusicbrainz, Enabled: settings.MusicBrainz},
			{Provider: apigen.MetadataProviderCoverartarchive, Enabled: settings.MusicBrainz},
		},
	}
}

// GetMetadataStatus answers the queue length and the state of every provider.
func (api *API) GetMetadataStatus(ctx context.Context, _ apigen.GetMetadataStatusRequestObject) (apigen.GetMetadataStatusResponseObject, error) {
	if api.metadata.Status == nil {
		problem := notImplementedProblem()

		return apigen.GetMetadataStatusdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
	}

	status, err := api.metadata.Status.Status(ctx)
	if err != nil {
		return nil, operationFailure(operationGetMetadataStatus, err, http.StatusServiceUnavailable)
	}

	return apigen.GetMetadataStatus200JSONResponse(metadataStatusResponse(status)), nil
}

func metadataStatusResponse(status enrich.Status) apigen.MetadataStatus {
	providers := make([]apigen.MetadataProviderStatus, 0, len(status.Providers))
	for _, provider := range status.Providers {
		providers = append(providers, apigen.MetadataProviderStatus{
			Provider: apigen.MetadataProvider(provider.Provider),
			State:    apigen.ProviderState(provider.State),
		})
	}

	return apigen.MetadataStatus{
		QueueLength: int32(min(max(status.QueueLength, 0), math.MaxInt32)),
		Providers:   providers,
	}
}
