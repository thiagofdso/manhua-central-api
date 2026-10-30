package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/providers"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/repository"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://manhua_user:manhua_secure_password_change_me@localhost:5432/manhua_pro?sslmode=disable"
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Falha ao conectar no PostgreSQL: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Banco de dados inacessível: %v", err)
	}

	repo := repository.NewPostgresCanonicalRepository(db)

	reg := providers.NewDefaultProviderRegistry("")

	log.Println("🚀 Iniciando sincronização do catálogo popular das plataformas...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	totalIngested := 0

	for _, d := range reg.ListDomains() {
		if d.Domain == "all" {
			continue
		}
		scraper, _ := reg.Get(d.Domain)
		log.Printf("📥 Coletando obras populares de %s (%s)...", d.Name, d.Domain)

		for page := 1; ; page++ {
			res, err := scraper.SearchOrBrowse(ctx, "", page)
			if err != nil {
				log.Printf("⚠️ Erro ao buscar página %d de %s: %v", page, d.Domain, err)
				break
			}

			if len(res.Items) == 0 {
				break
			}

			var batch []*domain.CanonicalManhua
			for _, it := range res.Items {
				slug := it.Slug
				if slug == "" {
					slug = strings.ToLower(strings.ReplaceAll(it.Title, " ", "-"))
				}
				cov := it.CoverURL

				batch = append(batch, &domain.CanonicalManhua{
					Slug:          slug,
					Title:         it.Title,
					CoverURL:      &cov,
					PrimaryDomain: it.PrimaryDomain,
					URLTemplate:   it.URLTemplate,
					LastChapter:   it.LastChapter,
				})
			}

			if err := repo.UpsertBatch(ctx, batch); err != nil {
				log.Printf("⚠️ Erro no UpsertBatch da página %d de %s: %v", page, d.Domain, err)
				break
			}

			totalIngested += len(batch)
			log.Printf("  ✅ Página %d: %d obras sincronizadas", page, len(batch))

			if page >= res.TotalPages {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	log.Printf("🎉 Sincronização concluída com sucesso! Total de obras processadas/atualizadas: %d\n", totalIngested)
}
