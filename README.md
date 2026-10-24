# Manhua Central — Backend Core API Gateway (Go)

Microsserviço central em Go (Golang) com arquitetura de alta performance:
- Proxy de streaming de imagens com zero buffer (`io.Copy` em chunks de 32KB) e proteção total contra SSRF.
- Autenticação unificada JWT e validação de Google ID Token (OAuth).
- Regras de negócio canônicas (Favoritos no topo, teto de lastChapter, ordenação atômica).
- Motor de cálculo Bayesiano para ratings comunitários.

## Executando Testes
```bash
go test -v -race ./...
```
