# Desafio Técnico — Programa Trainee Crawler/RPA & IA

Este projeto consiste em um Web Scraper de alta performance desenvolvido na linguagem Go, projetado com foco em concorrência segura, streaming de I/O estável, persistência relacional resiliente e integração com Inteligência Artificial (NLP).

---

## 🎁 Diferenciais e Bônus Implementados

1. **Persistência Relacional com Resiliência (PostgreSQL + Docker Compose):**
   Integração ativa com banco de dados PostgreSQL. A gravação é feita em lotes (*Bulk Ingestion*) de 100 em 100 livros de forma a mitigar sobrecargas de conexão e rede. O sistema possui resiliência a dados duplicados na origem utilizando restrição de unicidade baseada na URL estática do livro (`image_url UNIQUE`) e a cláusula `ON CONFLICT DO NOTHING`.
   
2. **Automação de Browser (Páginas Dinâmicas):**
   O arquivo `cmd/bonus_browser/main.go` demonstra capacidade de interagir com páginas renderizadas dinamicamente via JavaScript utilizando headless browser (`chromedp`), realizando a varredura do site *Quotes to Scrape (JS)* com validações de segurança contra ataques de SSRF e DNS Rebinding.

3. **Extração de Sinopses e NLP com IA (Gemini 2.5 Flash):**
   O arquivo `cmd/bonus_ai/main.go` realiza a raspagem dinâmica da URL do primeiro livro disponível na página inicial do site. Em seguida, extrai a sinopse longa e consome a API do Gemini 2.5 Flash para realizar análise de sentimento e extração de assunto principal, retornando uma estrutura estritamente validada em JSON.

4. **Pipeline CI/CD Otimizado com Cache Local:**
   O arquivo `.gitlab-ci.yml` configura o `GOPATH` localmente na pasta do projeto para permitir que o GitLab salve o cache de dependências de forma efetiva (`.go/pkg/mod/`), acelerando o tempo de execução do pipeline.

---

## 📂 Estrutura de Pastas do Projeto

```text
desafio/
├── cmd/
│   ├── scraper/
│   │   ├── main.go
│   │   └── main_test.go
│   ├── bonus_browser/
│   │   └── main.go
│   └── bonus_ai/
│       └── main.go
├── internal/
│   └── security/
│       └── ssrf.go
├── data/
│   ├── books.csv
│   └── books.jsonl
├── Dockerfile
├── docker-compose.yml
├── .gitlab-ci.yml
├── .gitignore
├── go.mod
└── go.sum
```

---

## 📊 Estrutura e Schema dos Dados Gerados

### 1. Banco de Dados (PostgreSQL)
A tabela `books` é instanciada automaticamente com o seguinte schema:
* `id`: SERIAL (Chave Primária)
* `title`: TEXT (Título do livro)
* `price`: NUMERIC (Preço decimal limpo)
* `rating`: INT (Nota mapeada de 1 a 5)
* `availability`: TEXT (Status de estoque)
* `image_url`: TEXT UNIQUE (URL estática e chave de unicidade)

### 2. Arquivos Planos (Pasta `data/`)
* **`books.csv`:** Arquivo plano estruturado com cabeçalho contendo `Title`, `Price`, `Rating`, `Availability` e `ImageURL`.
* **`books.jsonl`:** Formato JSON Lines (JSONL) onde cada linha é um objeto JSON independente e gravado de forma segura sem buffers manuais:

```jsonl
{
    "title": "A Light in the Attic",
    "price": 51.77,
    "rating": 3,
    "availability": "In stock",
    "image_url": "https://books.toscrape.com/catalogue/a-light-in-the-attic_1000/index.html"
}
```

---

## 💡 Decisões de Engenharia e Arquitetura

* **Consumo de Memória Estável O(1) (Streaming de I/O):**
  Para evitar o estouro de memória (OOM) comum em scrapers de grande volume, o robô não acumula os dados em fatias (*slices*) na memória RAM para salvar o JSON. O arquivo é processado na extensão `.jsonl` em formato JSON Lines, onde a gravação no disco local é feita linha por linha eliminando concatenações inseguras ou o uso de buffers que causam corrupção de arquivo em caso de encerramento anormal.
  
* **Mitigação de Syscalls (Bufferização de Escrita):**
  A gravação do arquivo JSON utiliza a biblioteca `bufio.NewWriter` para evitar a execução de chamadas de sistema (`write` do kernel) síncronas para cada livro. O sistema grava em um buffer de memória e realiza um único *Flush* final em lote. O CSV segue o mesmo fluxo de escrita bufferizada controlado via `csvWriter`.

* **Concorrência Segura (Pipeline de Canais):**
  Para paralelizar a requisição HTTP sem gerar condições de corrida (*data race*), o scraper opera de forma assíncrona (`colly.Async(true)`) com concorrência regulada por domínio (`Parallelism: 2`). A ingestão dos dados é feita através de um canal de dados bufferizado (`booksChan := make(chan Book, 100)`) consumido por uma única goroutine responsável pelo processamento de escrita (Thread-safe).

* **Segurança e Conformidade de Container (Docker):**
  O projeto foi adaptado para rodar com o usuário seguro estático não-root `appuser` (UID/GID 1000:1000) no Dockerfile, atendendo à conformidade de segurança de Pods do Kubernetes (`runAsNonRoot: true`), eliminando scripts de inicialização dinâmicos que exigiam privilégios de root temporários.

* **Proteção contra SSRF e DNS Rebinding:**
  No script do headless browser, implementou-se uma camada de validação estrita dos IPs resolvidos (`IsLoopback()`, `IsPrivate()`) para impedir o acesso a servidores internos da infraestrutura corporativa a partir de URLs fornecidas dinamicamente.

---

## 🚀 Como Executar o Projeto

### 1. Rodando com Docker Compose (Scraper + Postgres)
Com o Docker aberto na sua máquina, execute:
```bash
docker compose up --build
```
*Nota: O serviço PostgreSQL iniciará, aguardará a estabilização completa do banco através da verificação de `healthcheck` e iniciará o scraper. Os arquivos CSV e JSON serão gerados na pasta local `./data` automaticamente.*

### 2. Rodando Localmente (Sem Docker)
Necessita de Go 1.21+ instalado.

**2.1 Instale as dependências:**
```bash
go mod tidy
```

**2.2 Execute os testes de validação unitária e de integração:**
```bash
go test -v ./...
```

**2.3 Execute o robô principal:**
```bash
go run cmd/scraper/main.go
```
*(Ele salvará os arquivos no disco local em `data/` e, caso o Postgres esteja disponível na porta 5432, persistirá no banco de dados).*

### 3. Executando os Scripts de Bônus (IA e Browser)
* **Para rodar a automação dinâmica de navegador (Quotes to Scrape):**
```bash
go run cmd/bonus_browser/main.go
```
* **Para rodar a extração com IA e NLP:**
  Defina a variável com a sua chave do Gemini no ambiente e execute:
  *(No Linux/macOS)*
```bash
export GEMINI_API_KEY="sua_chave_aqui"
go run cmd/bonus_ai/main.go
```
  *(No Windows PowerShell)*
```powershell
$env:GEMINI_API_KEY="sua_chave_aqui"
go run cmd/bonus_ai/main.go
```

---

## ⚙️ Detalhamento da Esteira CI/CD

No arquivo `.gitlab-ci.yml`, a esteira executa as seguintes fases:
1. **`lint`:** Executa o linter oficial do Go (`golangci-lint`) garantindo conformidade com as boas práticas comunitárias e falhando o pipeline em caso de erros de estilo ou sintaxe.
2. **`test`:** Roda toda a suíte de testes de forma isolada, limpa e nativa usando mocks para o banco e para o servidor HTTP.
3. **`build`:** Realiza login seguro no GitLab Container Registry através das variáveis nativas do ambiente e envia a imagem gerada tagueada com o SHA do commit e `:latest` utilizando o cache otimizado do BuildKit.
4. **`deploy`:** Simula de forma programática o disparo do deploy na branch `main` para a AWS ECS com comandos reais simulados.

---

## 🔮 O que eu faria diferente com mais tempo (Melhorias de Produção)

Embora o robô esteja 100% funcional, estruturado em arquitetura concorrente e em conformidade com o edital do desafio, o processo de refatoração para um ambiente de produção corporativo real (Go-live) contemplaria as seguintes evoluções técnicas:

### 1. Otimização de Performance no Ingestão em Massa (Bulk Ingestion)
Atualmente, a persistência de dados em lote no banco PostgreSQL utiliza uma chamada de transação com preparação de query dinâmica (`PrepareContext`):
* **O que seria alterado:** Substituição do fluxo de preparação de instrução pela execução direta via `tx.ExecContext(ctx, queryStr, vals...)`.
* **Motivação:** Como o tamanho dos lotes de livros pode variar (ex: o último lote para limpar o canal pode conter menos de 100 livros), o formato da query string muda. Preparar instruções SQL cujo formato varia a cada chamada força o PostgreSQL a analisar e planejar planos de execução que nunca serão reutilizados. A remoção do `Prepare` elimina uma viagem de rede (RTT) extra por lote e evita o consumo inútil de memória de sessões do banco de dados.

### 2. Endurecimento de Segurança de Rede (Transport Encryption)
O banco de dados local utiliza a flag de ambiente padrão `sslmode=disable` para fins de sandbox de desenvolvimento:
* **O que seria alterado:** Em ambientes de Homologação (Staging) ou Produção dentro do AWS ECS, o parser de credenciais do banco exigiria estritamente conexões criptografadas de forma mandatória (`sslmode=require` ou `sslmode=verify-full`).
* **Motivação:** Mitigar o risco de interceptação de tráfego de dados e credenciais de acesso (*man-in-the-middle*) ao transitar informações fora de redes privadas isoladas.

### 3. Automação de Qualidade de Código (Lint & Format)
* **O que seria alterado:** Integração de ganchos de pré-commit locais (*Git Hooks*) executando `go fmt` e `goimports` antes de qualquer commit no Git.
* **Motivação:** Garantir a formatação visual e ordenação de imports estrita conforme o padrão oficial Go de forma 100% automatizada, eliminando pequenas imperfeições de identação na declaração de pacotes globais ou blocos de inicialização (como o mapa de ratings do scraper) antes do envio ao code review.

---

## 🤖 Uso da Inteligência Artificial Durante o Desafio

A inteligência artificial foi utilizada de forma estratégica e transparente como ferramenta de suporte técnico acelerado durante o desafio. Abaixo estão discriminadas as atividades e os **prompts exatos** utilizados durante a concepção:

| Atividade / Área | Prompt Exato Utilizado |
| :--- | :--- |
| **Configuração de Pipeline CI/CD e Cache** | *"Como mapear conceitualmente a infraestrutura de cache local (GOPATH) e salvar pacotes no GitLab CI/CD de forma eficiente para evitar downloads redundantes em cada build do Go?"* |
| **Segurança e Conformidade de Container** | *"Como configurar o Dockerfile do Go com a imagem Alpine para criar e rodar a aplicação sob um usuário não-root estático (UID/GID 1000) sem necessitar de privilégios de administrador temporários ou sudo?"* |
| **Validação de Sintaxe e Concorrência** | *"Como garantir o fechamento seguro de canais bufferizados usando o Colly de forma assíncrona, evitando que goroutines fiquem vazadas ou travadas esperando pacotes se o contexto do sistema operacional for interrompido?"* |

A interação com as inteligências artificiais focou exclusivamente na resolução de complexidades sintáticas do ecossistema Go, otimização de caches na infraestrutura de CI e segurança de containers (K8s Compliance), garantindo que toda a arquitetura de dados e lógica principal do crawler fossem de autoria própria do candidato.