package telegram

import "strings"

var markdownV2Escapes = []string{"_", "*", "[", "]", "(", ")", "~", "`", ">", "#", "+", "-", "=", "|", "{", "}", ".", "!"}

func EscapeMarkdownV2(input string) string {
	result := input
	for _, token := range markdownV2Escapes {
		result = strings.ReplaceAll(result, token, "\\"+token)
	}
	return result
}
