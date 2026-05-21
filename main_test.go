package main

import (
	"testing"
)

// DICA DE OURO: Isso se chama "Table-Driven Tests". 
// É o padrão mais respeitado em Go para fazer testes unitários.

func TestCleanPrice(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected float64
	}{
		{"Preço normal", "£12.99", 12.99},
		{"Preço com bug de encoding", "Â£45.17", 45.17},
		{"String vazia", "", 0.0},
		{"Texto invalido", "grátis", 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanPrice(tt.input)
			if result != tt.expected {
				t.Errorf("Esperado %f, mas recebeu %f", tt.expected, result)
			}
		})
	}
}

func TestMapRating(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"Uma estrela", "star-rating One", 1},
		{"Tres estrelas", "star-rating Three", 3},
		{"Cinco estrelas", "star-rating Five", 5},
		{"Classe invalida", "star-rating Invalido", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapRating(tt.input)
			if result != tt.expected {
				t.Errorf("Esperado %d, mas recebeu %d", tt.expected, result)
			}
		})
	}
}