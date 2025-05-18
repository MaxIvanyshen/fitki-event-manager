package config

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"log/slog"
	"os"
	"strconv"
	"sync"

	"giveaway-tool/database/sqlc"
)

type Config struct {
	CurrentEventID *int64 `json:"current_event_id"`
}

var (
	configInstance *Config
	configFile     = "config.json"
	mutex          sync.Mutex
)

func InitConfig(ctx context.Context, queries *sqlc.Queries) {
	configInstance = &Config{}

	// Try to load from file first
	if err := loadConfigFromFile(); err != nil {
		// If file doesn't exist or has issues, fall back to env var
		currentEventID := os.Getenv("CURRENT_EVENT_ID")
		if currentEventID != "" {
			eventID, err := strconv.ParseInt(currentEventID, 10, 64)
			if err != nil {
				panic("Invalid CURRENT_EVENT_ID value")
			}
			configInstance.CurrentEventID = &eventID
			// Save to file for persistence
			saveConfigToFile()
		}
	}

	if configInstance.CurrentEventID == nil {
		event, err := queries.GetLastEvent(ctx)
		if err != nil {
			slog.LogAttrs(ctx, slog.LevelError, "Failed to get last event from database", slog.Any("error", err))
			return
		}
		if event != nil {
			configInstance.CurrentEventID = &event.ID
			// Save to file for persistence
			if err := saveConfigToFile(); err != nil {
				slog.LogAttrs(ctx, slog.LevelError, "Failed to save config to file", slog.Any("error", err))
			}
		} else {
			slog.LogAttrs(ctx, slog.LevelInfo, "No events found in database")
		}
	}
}

func GetConfig() *Config {
	mutex.Lock()
	defer mutex.Unlock()

	if configInstance == nil {
	}
	return configInstance
}

func GetCurrentEventID() int64 {
	if configInstance == nil || configInstance.CurrentEventID == nil {
		return 0
	}
	return *configInstance.CurrentEventID
}

func SetCurrentEventID(eventID int64) {
	mutex.Lock()
	defer mutex.Unlock()

	// Update environment variable (optional, for backward compatibility)
	err := os.Setenv("CURRENT_EVENT_ID", strconv.FormatInt(eventID, 10))
	if err != nil {
		slog.LogAttrs(context.Background(), slog.LevelError, "Failed to set CURRENT_EVENT_ID", slog.String("error", err.Error()))
	}

	// Update in-memory config
	if configInstance == nil {
		configInstance = &Config{}
	}
	configInstance.CurrentEventID = &eventID

	// Save to file for persistence
	if err := saveConfigToFile(); err != nil {
		slog.LogAttrs(context.Background(), slog.LevelError, "Failed to save config to file", slog.String("error", err.Error()))
	} else {
		slog.LogAttrs(context.Background(), slog.LevelInfo, "Set CURRENT_EVENT_ID", slog.Int64("event_id", *configInstance.CurrentEventID))
	}
}

func loadConfigFromFile() error {
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, configInstance)
}

func saveConfigToFile() error {
	data, err := json.MarshalIndent(configInstance, "", "  ")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(configFile, data, 0644)
}
