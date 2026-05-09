package telegram

import (
	"errors"
	"fmt"
	"strings"
)

const (
	errUnauthorizedMessage = "telegram authorization failed"
	errUnauthorizedChat    = "%w: telegram chat %d is not authorized"
	errUnauthorizedUser    = "%w: telegram user %d is not authorized"
)

var errUnauthorizedTelegramCommand = errors.New(errUnauthorizedMessage)

type Authorization struct {
	ChatID         int64
	AllowedUserIDs map[int64]bool
}

func (auth Authorization) Check(chatID int64, userID int64) error {
	if chatID != auth.ChatID {
		return fmt.Errorf(errUnauthorizedChat, errUnauthorizedTelegramCommand, chatID)
	}
	if !auth.AllowedUserIDs[userID] {
		return fmt.Errorf(errUnauthorizedUser, errUnauthorizedTelegramCommand, userID)
	}
	return nil
}

type ParsedCommand struct {
	Name string
	Args []string
}

func ParseCommand(text string) ParsedCommand {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return ParsedCommand{}
	}
	return ParsedCommand{Name: fields[0], Args: fields[1:]}
}
