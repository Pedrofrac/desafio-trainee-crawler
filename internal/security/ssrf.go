package security

import (
	"context"
	"fmt"
	"net"
	"net/url"
)

// ValidateTargetURL resolve o DNS no Go (ToC) usando contexto de timeout e gera uma regra estrita contra DNS Rebinding (TOCTOU)
func ValidateTargetURL(ctx context.Context, rawURL string) (string, string, error) {
	if rawURL == "" {
		return "http://quotes.toscrape.com/js/", "MAP quotes.toscrape.com 104.21.68.42", nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("URL sintaticamente invalida: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", "", fmt.Errorf("esquema de protocolo nao seguro (%s)", parsed.Scheme)
	}

	host := parsed.Hostname()
	
	// RESOLUÇÃO SEGURA: Utiliza o resolvedor nativo com suporte a contexto para evitar travamentos infinitos
	ips, err := net.DefaultResolver.LookupIPContext(ctx, "ip", host)
	if err != nil {
		return "", "", fmt.Errorf("falha ao resolver DNS do host com contexto: %w", err)
	}
	if len(ips) == 0 {
		return "", "", fmt.Errorf("nenhum IP retornado para o host: %s", host)
	}

	// SEGURANÇA: Valida TODOS os IPs retornados pelo DNS para evitar desvios por multi-homed DNS
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsPrivate() {
			return "", "", fmt.Errorf("bloqueio de seguranca: conexao para IP local ou privado nao permitida (%s)", ip.String())
		}
	}

	// Retorna a regra de mapeamento estrita para injetar no Chromium (--host-resolver-rules)
	rule := fmt.Sprintf("MAP %s %s", host, ips[0].String())
	return rawURL, rule, nil
}
