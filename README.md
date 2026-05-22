# Desafio Técnico — Programa Trainee Crawler/RPA & IA

Este projeto consiste em um Web Scraper de dados estruturados desenvolvido na linguagem Go, projetado com foco em arquitetura concorrente baseada em buffers, streaming de I/O, persistência relacional e integração com Inteligência Artificial (NLP).

---

## 📂 Arquitetura de Concorrência e Pipeline (Fan-Out Desacoplado)

Para mitigar o gargalo físico de rede, o pipeline de concorrência foi estruturado sob uma arquitetura de buffers e desacoplamento de fluxo:

1. **Prioridade ao Disco Plano:** A goroutine consumidora principal lê do canal `booksChan` e realiza a escrita síncrona imediata nos arquivos locais (`books.csv` e `books.jsonl`). A latência do banco de dados não afeta a escrita local.
2. **Buffer de Persistência:** A gravação relacional no PostgreSQL ocorre de forma assíncrona por meio do canal intermediário `dbChan` (capacidade de 1000 mensagens).
3. **Mecanismo Fail-Safe (Backpressure Zero-Blocking):** Caso o banco de dados enfrente lentidão ou indisponibilidade e o buffer do `dbChan` sature, o fluxo entra em um desvio não-bloqueante (`select default`) que encaminha os registros diretamente para a DLQ (`dlqChan`). Isso evita o congelamento da goroutine consumidora de disco plano.
4. **Encerramento Determinístico (Graceful Shutdown):** O encerramento respeita a ordem de concorrência: interrupção do produtor (Colly) -> dreno dos buffers de escrita de arquivos plano -> fechamento do `dbChan` -> dreno e persistência do banco -> fechamento e sincronização da DLQ -> sincronização física dos descritores de arquivo com o kernel do S.O. (`Sync()`).

---

## 🎁 Diferenciais e Bônus Implementados

1. **Persistência Relacional com Resiliência (PostgreSQL + Bulk Ingestion):**
   Integração ativa com banco de dados PostgreSQL. A gravação é feita em lotes (*Bulk Ingestion*) de 100 em 100 livros de forma a mitigar sobrecargas de conexão e rede. O sistema possui resiliência a dados duplicados na origem utilizando restrição de unicidade baseada na URL estática do livro (`image_url UNIQUE`) e a cláusula `ON CONFLICT DO NOTHING`.
   
2. **Automação de Browser (Páginas Dinâmicas):**
   O arquivo `cmd/bonus_browser/main.go` demonstra capacidade de interagir com páginas renderizadas dinamicamente via JavaScript utilizando headless browser (`chromedp`), realizando a varredura do site *Quotes to Scrape (JS)* com validações de segurança contra ataques de SSRF e DNS Rebinding por meio do arquivo `internal/security/ssrf.go`.

3. **Extração de Sinopses e NLP com IA (Gemini 2.5 Flash):**
   O arquivo `cmd/bonus_ai/main.go` realiza a raspagem dinâmica da URL do primeiro livro disponível na página inicial do site. Em seguida, extrai a sinopse longa e consome a API do Gemini 2.5 Flash para realizar análise de sentimento e extração de assunto principal, retornando uma estrutura estritamente validada em JSON.

4. **Mecanismo de Fallback Sintático para JSON da IA (State Machine):**
   O método `cleanJSONFallback` utiliza uma varredura de caracteres (*State Machine*) ciente de contexto de strings. Ele lê o fluxo bruto da LLM e conta chaves `{}` apenas quando está fora de strings literais. Isso impede falhas de parsing sintático caso a IA retorne chaves estruturais como conteúdo literal da sinopse do livro.

5. **Hashing Criptográfico em Conformidade SecOps:**
   Para registros com URLs de imagem ausentes, o sistema gera chaves de unicidade seguras utilizando hashing de 256 bits via padrão de mercado **SHA-256** (`crypto/sha256`), atendendo a políticas de segurança corporativas contra colisões (evitando o uso de hashes obsoletos como o MD5).

6. **Esteira CI/CD com Cache Local de Dependências:**
   O arquivo `.gitlab-ci.yml` configura o `GOPATH` localmente na pasta do projeto para permitir que o GitLab salve o cache de dependências de forma efetiva (`.go/pkg/mod/`). Observa-se que este cache otimiza estritamente a execução da etapa de testes unitários (`test`). A etapa de build ocorre em container isolado via Docker-in-Docker (`docker:dind`), dependendo estritamente do cache de camadas do Docker (BuildKit) e não herdando o GOPATH do host.

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

| Atividade / Área | Prompt Exato Utilizado |
| :--- | :--- |
| **Revisão Geral / Prompt Mestre** | *"Você é um Engenheiro de Software Sênior, Especialista em Segurança e Revisor Técnico de Código extremamente rigoroso. Seu papel é fazer uma auditoria implacável no projeto de desafio técnico de um candidato à vaga de 'Trainee Crawler/RPA & IA'."* |
| **Configuração de Pipeline CI/CD e Cache** | *"Como mapear conceitualmente a infraestrutura de cache local (GOPATH) e salvar pacotes no GitLab CI/CD de forma eficiente para evitar downloads redundantes em cada build do Go?"* |
| **Segurança e Conformidade de Container** | *"Como configurar o Dockerfile do Go com a imagem Alpine para criar e rodar a aplicação sob um usuário não-root estático (UID/GID 1000) sem necessitar de privilégios de administrador temporários ou sudo?"* |
| **Validação de Sintaxe e Concorrência** | *"Como garantir o fechamento seguro de canais bufferizados usando o Colly de forma assíncrona, evitando que goroutines fiquem vazadas ou travadas esperando pacotes se o contexto do sistema operacional for interrompido?"* |
| **Limpeza de Dados e Parsing** | *"Qual a forma mais performática e menos frágil de processar strings de preços com símbolos de moedas variados e separadores de milhar em Go, evitando erros de conversão float?"* |
| **Testes de Integração com Mocks** | *"Como estruturar testes unitários em Go utilizando a biblioteca sqlmock para validar uma função que executa inserções em massa (Bulk Insert) dentro de uma transação?"* |
| **Integração com LLM (NLP)** | *"Como garantir que o Gemini 2.5 Flash retorne estritamente um JSON válido ao analisar uma sinopse, e como lidar com respostas que venham envolvidas em blocos de Markdown?"* |

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
### ⚠️ Solução de Problemas no Windows (Bônus de Navegador)

Se ao rodar o bônus de navegador (`go run cmd/bonus_browser/main.go`) você encontrar erros de inicialização ou limite de tempo esgotado (*timeout*), aplique estas duas soluções simples de ambiente local:

1. **Chrome não encontrado no PATH:** O Windows não acha o executável do Chrome sozinho. Execute este comando no PowerShell ativo antes de rodar o programa para corrigir:
   ```powershell
   $env:Path += ";C:\Program Files\Google\Chrome\Application"
   ```

2. **Timeout devido a Bloqueio de IP/DNS:** O código original utiliza uma regra estrita de segurança para travar o site `quotes.toscrape.com` em um IP fixo da Cloudflare (`104.21.68.42`). Em redes domésticas ou sob firewalls corporativos no Windows, essa injeção direta de IP é bloqueada pelo sistema, fazendo o Chrome carregar infinitamente até dar *timeout*.
   * **Como foi resolvido:** No arquivo `cmd/bonus_browser/main.go`, desativamos essa regra estrita comentando a flag `host-resolver-rules` do Chrome e as referências à variável `dnsRule`, permitindo que o navegador resolva a conexão utilizando a rede padrão do Windows.


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

## ⚙️ Limitações Técnicas Conhecidas

* **Precisão de Locale e Risco de Corrupção de Dados:** A heurística de parsing monetário em `cleanPrice` diferencia milhares de decimais baseando-se no comprimento de caracteres após o separador isolado (`afterDot != 3`). Dessa forma, moedas ou índices que utilizem estritamente precisão de 3 casas decimais (ex: precificação de combustíveis ou ações onde `1.250` representa `1.25` e não `1250.0`) **não são suportados** por este motor e resultarão em interpretação inadequada do valor numérico.
Esta função é uma heurística proprietária baseada em suposições simplificadas de escopo e não substitui uma biblioteca padrão de internacionalização (como `golang.org/x/text/currency`). A aplicação direta deste parser em fontes de dados com padrões de locale dinâmicos trará riscos de gravação de valores monetários corrompidos no banco de dados.

---

## 🔮 O que eu faria diferente com mais tempo (Melhorias de Produção)

### 1. Protocolo Nativo PostgreSQL COPY
Substituição do algoritmo de geração dinâmica de queries de inserção concorrente (`INSERT INTO ... VALUES ...`) pelo mecanismo nativo **`PostgreSQL COPY`** (por meio do driver de mercado `pgx`). O uso do comando de carregamento em massa reduz o processamento de queries sintáticas e otimiza o uso de recursos do servidor relacional.

### 2. Endurecimento de Segurança de Rede (Transport Encryption)
O banco de dados local utiliza a flag de ambiente padrão `sslmode=disable` para fins de sandbox de desenvolvimento:
* **O que seria alterado:** Em ambientes de Homologação (Staging) ou Produção dentro do AWS ECS, o parser de credenciais do banco exigiria estritamente conexões criptografadas de forma mandatória (`sslmode=require` ou `sslmode=verify-full`).
* **Motivação:** Mitigar o risco de interceptação de tráfego de dados e credenciais de acesso (*man-in-the-middle*) ao transitar informações fora de redes privadas isoladas.

### 3. Automação de Qualidade de Código (Lint & Format)
* **O que seria alterado:** Integração de ganchos de pré-commit locais (*Git Hooks*) executando `go fmt` e `goimports` antes de qualquer commit no Git.
* **Motivação:** Garantir a formatação visual e ordenação de imports estrita conforme o padrão oficial Go de forma 100% automatizada, eliminando imperfeições de identação na declaração de pacotes globais ou blocos de inicialização (como o mapa de ratings do scraper) antes do envio ao code review.

---

## 🤖 Uso da Inteligência Artificial Durante o Desafio

A inteligência artificial foi utilizada de forma estratégica e transparente como ferramenta de suporte técnico acelerado durante o desafio. Abaixo estão discriminadas as atividades e os **prompts exatos** utilizados durante a concepção:

| Atividade / Área | Prompt Exato Utilizado |
| :--- | :--- |
| **Revisão Geral / Prompt Mestre** | *"Você é um Engenheiro de Software Sênior, Especialista em Segurança e Revisor Técnico de Código extremamente rigoroso. Seu papel é fazer uma auditoria implacável no projeto de desafio técnico de um candidato à vaga de 'Trainee Crawler/RPA & IA'."* |
| **Configuração de Pipeline CI/CD e Cache** | *"Como mapear conceitualmente a infraestrutura de cache local (GOPATH) e salvar pacotes no GitLab CI/CD de forma eficiente para evitar downloads redundantes em cada build do Go?"* |
| **Segurança e Conformidade de Container** | *"Como configurar o Dockerfile do Go com a imagem Alpine para criar e rodar a aplicação sob um usuário não-root estático (UID/GID 1000) sem necessitar de privilégios de administrador temporários ou sudo?"* |
| **Validação de Sintaxe e Concorrência** | *"Como garantir o fechamento seguro de canais bufferizados usando o Colly de forma assíncrona, evitando que goroutines fiquem vazadas ou travadas esperando pacotes se o contexto do sistema operacional for interrompido?"* |
| **Limpeza de Dados e Parsing** | *"Qual a forma mais performática e menos frágil de processar strings de preços com símbolos de moedas variados e separadores de milhar em Go, evitando erros de conversão float?"* |
| **Testes de Integração com Mocks** | *"Como estruturar testes unitários em Go utilizando a biblioteca sqlmock para validar uma função que executa inserções em massa (Bulk Insert) dentro de uma transação?"* |
| **Integração com LLM (NLP)** | *"Como garantir que o Gemini 2.5 Flash retorne estritamente um JSON válido ao analisar uma sinopse, e como lidar com respostas que venham envolvidas em blocos de Markdown?"* |

A interação com as inteligências artificiais focou exclusivamente na resolução de complexidades sintáticas do ecossistema Go, otimização de caches na infraestrutura de CI e segurança de containers (K8s Compliance), garantindo que toda a arquitetura de dados e lógica principal do crawler fossem de autoria própria do candidato.
