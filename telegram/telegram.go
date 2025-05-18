package telegram

import (
	"context"
	"database/sql"
	"fmt"
	"giveaway-tool/config"
	"giveaway-tool/database/sqlc"
	"log/slog"
	"os"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

const REGISTERED_ERROR = "pq: duplicate key value violates unique constraint \"unique_tg_event_id\""

type State int64

const (
	_ State = iota
	Started
	WaitingForName
	Done
)

type StateKey struct {
	ChatID  int64
	EventID int64
}

type Service struct {
	mu             sync.Mutex
	logger         *slog.Logger
	queries        *sqlc.Queries
	bot            *tgbotapi.BotAPI
	welcomeMessage string
	state          map[StateKey]State
}

func Start(ctx context.Context, logger *slog.Logger, db *sql.DB) {
	queries := sqlc.New(db)
	bot, err := tgbotapi.NewBotAPI(os.Getenv("TELEGRAM_BOT_TOKEN"))

	if err != nil {
		logger.LogAttrs(nil, slog.LevelError, "Failed to create Telegram bot", slog.Any("error", err))
		return
	}

	currentEventID := config.GetCurrentEventID()

	svc := &Service{
		logger:  logger,
		queries: queries,
		bot:     bot,
		state:   make(map[StateKey]State),
	}

	event, err := svc.queries.GetEventByID(ctx, currentEventID)
	if err != nil {
		logger.LogAttrs(ctx, slog.LevelError, "Failed to get event by ID", slog.Any("error", err))
	}

	svc.welcomeMessage = fmt.Sprintf("Привіт! Я бот для реєстрації на івент ФІТКІ \"%s\".\n\nВведи своє прізвище та ім'я, щоб зареєструватися.", event.Name)

	go svc.run(ctx)

	svc.logger.LogAttrs(ctx, slog.LevelInfo, "Telegram service started")
}

func (s *Service) run(ctx context.Context) {
	updates, err := s.bot.GetUpdatesChan(tgbotapi.UpdateConfig{})
	if err != nil {
		s.logger.LogAttrs(nil, slog.LevelError, "Failed to get updates channel", slog.Any("error", err))
		return
	}

	for update := range updates {
		go s.processUpdate(ctx, update)
	}
}

func (s *Service) processUpdate(ctx context.Context, update tgbotapi.Update) {
	if update.Message == nil {
		return
	}

	state := s.getState(update.Message.Chat.ID)

	s.logger.LogAttrs(ctx, slog.LevelInfo, "Received message", slog.Any("message", update.Message.Text))

	var msg tgbotapi.MessageConfig

	switch state {
	case Started:
		msg = tgbotapi.NewMessage(update.Message.Chat.ID, s.welcomeMessage)
		s.setState(update.Message.Chat.ID, WaitingForName)
	case WaitingForName:
		if update.Message.Text == "/start" {
			msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Вже чекаю на твоє ім'я!")
		} else {
			if _, err := s.queries.CreateUser(ctx, &sqlc.CreateUserParams{
				TgID:     int64(update.Message.From.ID),
				Name:     update.Message.Text,
				Username: update.Message.From.UserName,
				EventID:  config.GetCurrentEventID(),
			}); err != nil {
				s.logger.LogAttrs(ctx, slog.LevelError, "Failed to create user", slog.Any("error", err))
				if err.Error() == REGISTERED_ERROR {
					msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Ти вже зареєстрований!")
				} else {
					msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Сталася помилка. Спробуй ще раз.")
				}
			} else {
				msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Дякую! Ти успішно зареєстрований.")
				s.setState(update.Message.Chat.ID, Done)
			}
			s.setState(update.Message.Chat.ID, Done)
		}
	case Done:
		msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Ти вже зареєстрований!")
	}
	msg.ParseMode = tgbotapi.ModeMarkdown
	if _, err := s.bot.Send(msg); err != nil {
		s.logger.LogAttrs(ctx, slog.LevelError, "Failed to send message", slog.Any("error", err))
	}
	return
}

func (s *Service) getState(chatID int64) State {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := StateKey{
		ChatID:  chatID,
		EventID: config.GetCurrentEventID(),
	}

	if state, ok := s.state[key]; ok {
		return state
	}

	return Started
}

func (s *Service) setState(chatID int64, state State) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := StateKey{
		ChatID:  chatID,
		EventID: config.GetCurrentEventID(),
	}

	s.state[key] = state
}
