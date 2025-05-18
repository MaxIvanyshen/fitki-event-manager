package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"giveaway-tool/config"
	"giveaway-tool/database"
	"giveaway-tool/database/sqlc"
	"giveaway-tool/service"
	"giveaway-tool/telegram"

	"github.com/joho/godotenv"
)

func main() {
	ctx := context.TODO()
	logger := slog.Default()
	router := http.NewServeMux()
	err := godotenv.Load()
	if err != nil {
		logger.LogAttrs(ctx, slog.LevelError, "Failed to load .env file", slog.Any("error", err))
	}

	db, err := database.New(ctx)
	if err != nil {
		logger.LogAttrs(ctx, slog.LevelError, "Failed to connect to database", slog.Any("error", err))
		return
	}

	config.InitConfig(ctx, sqlc.New(db))
	logger.LogAttrs(ctx, slog.LevelInfo, "Current event ID", slog.Int64("event_id", config.GetCurrentEventID()))

	service.Start(router, logger, db)
	telegram.Start(ctx, logger, db)

	port := os.Getenv("PORT")

	logger.LogAttrs(ctx, slog.LevelInfo, "Starting server", slog.String("port", port))
	if err := http.ListenAndServe(fmt.Sprintf(":%s", port), router); err != nil {
		logger.LogAttrs(ctx, slog.LevelError, "Failed to start server", slog.Any("error", err))
		return
	}
}
