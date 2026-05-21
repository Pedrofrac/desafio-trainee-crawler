import os

# Lista atualizada com os novos nomes de pastas do padrao /cmd e sem o entrypoint.sh deletado
arquivos_projeto = [
    "cmd/scraper/main.go",
    "cmd/scraper/main_test.go",
    "cmd/bonus_browser/main.go",
    "cmd/bonus_ai/main.go",
    "internal/security/ssrf.go",
    "Dockerfile",
    "docker-compose.yml",
    ".gitlab-ci.yml",
    ".gitignore",
    "README.md"
]

nome_saida = "codigo_projeto.txt"

print("🔍 Iniciando mesclagem dos arquivos do projeto...")

try:
    with open(nome_saida, "w", encoding="utf-8") as arquivo_final:
        for caminho_arq in arquivos_projeto:
            if os.path.exists(caminho_arq):
                # Pega apenas o nome do arquivo para o cabecalho
                nome_simples = os.path.basename(caminho_arq)
                # Se for main.go, mantem o caminho da pasta no cabecalho para a outra IA nao se confundir
                if nome_simples == "main.go" or nome_simples == "main_test.go":
                    pasta_pai = os.path.basename(os.path.dirname(caminho_arq))
                    cabecalho = f"cmd/{pasta_pai}/{nome_simples}"
                else:
                    cabecalho = nome_simples

                print(f"📄 Lendo: {caminho_arq}")
                
                # Escreve o cabecalho com o nome correto do arquivo
                arquivo_final.write(f"\n=== {cabecalho} ===\n\n")
                
                # Le o conteudo do arquivo e escreve no unificado
                with open(caminho_arq, "r", encoding="utf-8") as arq_leitura:
                    conteudo = arq_leitura.read()
                    arquivo_final.write(conteudo)
                    
                arquivo_final.write("\n\n" + "="*30 + "\n")
            else:
                print(f"⚠️  Aviso: O arquivo '{caminho_arq}' não foi encontrado e será pulado.")
                
    print(f"\n🎉 Sucesso! Tudo foi unificado no arquivo: '{nome_saida}'")
    print("Agora basta copiar todo o conteúdo dele e colar na outra IA!")

except Exception as e:
    print(f"❌ Erro ao mesclar os arquivos: {e}")