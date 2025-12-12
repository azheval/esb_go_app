package i18n

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"golang.org/x/text/message/catalog"
)

// Service manages internationalization.
type Service struct {
	logger      *slog.Logger
	catalog     catalog.Catalog
	acceptRange language.Matcher
}

// NewService creates a new i18n service.
func NewService(localesDir string, logger *slog.Logger) (*Service, error) {
	// Use English as the fallback language.
	builder := catalog.NewBuilder(catalog.Fallback(language.English))

	supportedLangs := []language.Tag{language.English}

	// Load translations from JSON files
	files, err := os.ReadDir(localesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read locales directory: %w", err)
	}

	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
			langStr := strings.TrimSuffix(file.Name(), ".json")
			langTag, err := language.Parse(langStr)
			if err != nil {
				logger.Warn("failed to parse language tag from file name", "file", file.Name(), "error", err)
				continue
			}
			// Avoid re-adding English if en.json exists
			if langTag != language.English {
				supportedLangs = append(supportedLangs, langTag)
			}

			filePath := filepath.Join(localesDir, file.Name())
			data, err := os.ReadFile(filePath)
			if err != nil {
				logger.Error("failed to read translation file", "file", filePath, "error", err)
				continue
			}

			translations := make(map[string]string)
			if err := json.Unmarshal(data, &translations); err != nil {
				logger.Error("failed to unmarshal translation file", "file", filePath, "error", err)
				continue
			}

			for key, value := range translations {
				if err := builder.SetString(langTag, key, value); err != nil {
					logger.Error("failed to set string for language", "lang", langTag, "key", key, "error", err)
				}
			}
			logger.Info("loaded translations", "language", langTag.String(), "file", file.Name())
		}
	}

	if len(supportedLangs) == 0 {
		return nil, fmt.Errorf("no translation files found in %s", localesDir)
	}

	return &Service{
		logger:      logger,
		catalog:     builder,
		acceptRange: language.NewMatcher(supportedLangs),
	}, nil
}

// GetPrinter returns a message.Printer for the best matching language based on Accept-Language header.
func (s *Service) GetPrinter(acceptLanguage string) *message.Printer {
	tags, _, err := language.ParseAcceptLanguage(acceptLanguage)
	if err != nil {
		s.logger.Warn("failed to parse Accept-Language header, falling back to default", "header", acceptLanguage, "error", err)
		return message.NewPrinter(language.English, message.Catalog(s.catalog)) // Fallback to English
	}

	bestMatch, _, _ := s.acceptRange.Match(tags...)
	return message.NewPrinter(bestMatch, message.Catalog(s.catalog))
}

// Sprintf formats and translates a string using the best matching language.
func (s *Service) Sprintf(acceptLanguage, key string, args ...interface{}) string {
	printer := s.GetPrinter(acceptLanguage)
	return printer.Sprintf(key, args...)
}

// SprintfWithTag formats and translates a string using a specific language tag.
func (s *Service) SprintfWithTag(langTag language.Tag, key string, args ...interface{}) string {
	printer := message.NewPrinter(langTag, message.Catalog(s.catalog))
	return printer.Sprintf(key, args...)
}
