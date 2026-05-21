package main

import (
	"encoding/csv"
	"os"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

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

// Teste de Integracao Concorrente usando Mock (Correcao definitiva de compilacao)
func TestStartPipelineWithMock(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Erro ao iniciar mock: %s", err)
	}
	defer db.Close()

	// Define as expectativas exatas do fluxo de transacao no banco
	mock.ExpectBegin()
	prep := mock.ExpectPrepare("INSERT INTO books")
	prep.ExpectExec().
		WithArgs("Livro de Teste", 15.50, 4, "In stock", "http://imagem.com/teste.jpg").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	// Arquivos temporarios seguros para evitar a criacao de lixo no disco local de testes
	fCSV, err := os.CreateTemp("", "test_csv")
	if err != nil {
		t.Fatalf("Erro ao criar CSV temporario: %s", err)
	}
	fJSON, err := os.CreateTemp("", "test_json")
	if err != nil {
		t.Fatalf("Erro ao criar JSON temporario: %s", err)
	}
	defer fCSV.Close()
	defer fJSON.Close()
	defer os.Remove(fCSV.Name())
	defer os.Remove(fJSON.Name())

	csvWriter := csv.NewWriter(fCSV)

	// Inicializa a pipeline concorrente real usando o mock do banco de dados
	booksChan, wg := startPipeline(db, csvWriter, fJSON)

	// Injeta o dado de teste pelo canal
	booksChan <- Book{
		Title:        "Livro de Teste",
		Price:        15.50,
		Rating:       4,
		Availability: "In stock",
		ImageURL:     "http://imagem.com/teste.jpg",
	}

	// Fecha o canal para iniciar o encerramento ordenado da pipeline de gravacao
	close(booksChan)
	wg.Wait()

	// Verifica se todas as queries esperadas pelo banco de dados foram executadas
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Expectativas do banco nao atendidas: %s", err)
	}
}
