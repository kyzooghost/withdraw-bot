package telegram

import "testing"

func TestEscapeMarkdownV2EscapesDynamicValue(t *testing.T) {
	// Arrange
	input := "share_price_loss: WARN!"

	// Act
	result := EscapeMarkdownV2(input)

	// Assert
	expected := "share\\_price\\_loss: WARN\\!"
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}

func TestEscapeMarkdownV2EscapesReservedCharacters(t *testing.T) {
	// Arrange
	input := "_*[]()~`>#+-=|{}.!"

	// Act
	result := EscapeMarkdownV2(input)

	// Assert
	expected := "\\_\\*\\[\\]\\(\\)\\~\\`\\>\\#\\+\\-\\=\\|\\{\\}\\.\\!"
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}

func TestEscapeMarkdownV2EscapesBackslash(t *testing.T) {
	// Arrange
	input := `C:\vault\share_price_loss`

	// Act
	result := EscapeMarkdownV2(input)

	// Assert
	expected := `C:\\vault\\share\_price\_loss`
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}
