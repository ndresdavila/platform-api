package main

import (
	"log"
	"net/http"

	"github.com/ndresdavila/platform-api/internal/platform"
)

func main() {
	cfg := platform.LoadConfig()
	handler, err := platform.NewServer(cfg)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("platform-api listening on %s → core %s", cfg.HTTPAddr, cfg.CoreBaseURL)
	log.Fatal(http.ListenAndServe(cfg.HTTPAddr, handler))
}
