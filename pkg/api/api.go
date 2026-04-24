package api

import (
	"context"
	"crypto/tls"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/zzci/httpreq/pkg/httpreq"
	"github.com/zzci/httpreq/web"

	"github.com/caddyserver/certmagic"
	"github.com/julienschmidt/httprouter"
	"github.com/rs/cors"
	"go.uber.org/zap"
)

type API struct {
	Config  *httpreq.Config
	DB      httpreq.DB
	Logger  *zap.SugaredLogger
	Version string
	errChan chan error
}

func Init(config *httpreq.Config, db httpreq.DB, logger *zap.SugaredLogger, errChan chan error, version string) API {
	a := API{Config: config, DB: db, Logger: logger, Version: version, errChan: errChan}
	return a
}

func (a *API) Start(dnsservers []httpreq.NS) {
	var err error
	stderrorlog, err := zap.NewStdLogAt(a.Logger.Desugar(), zap.ErrorLevel)
	if err != nil {
		a.errChan <- err
		return
	}
	router := httprouter.New()
	c := cors.New(cors.Options{
		AllowedOrigins:     a.Config.API.CorsOrigins,
		AllowedMethods:     []string{"GET", "POST", "DELETE", "OPTIONS"},
		AllowedHeaders:     []string{"Authorization", "Content-Type"},
		OptionsPassthrough: false,
		Debug:              a.Config.General.Debug,
	})
	if a.Config.General.Debug {
		c.Log = stderrorlog
	}

	// httpreq endpoints (lego DNS provider) — Basic Auth
	router.POST("/present", a.BasicAuthHTTPreq(a.webPresentPost))
	router.POST("/cleanup", a.BasicAuthHTTPreq(a.webCleanupPost))

	// Health check and llms.txt
	router.GET("/health", a.healthCheck)
	router.GET("/llms.txt", a.serveLLMsTxt)

	// Web API endpoints — JWT auth
	router.POST("/api/register", a.apiRegister)
	router.POST("/api/login", a.apiLogin)
	router.GET("/api/info", a.apiInfo)
	router.GET("/api/profile", a.JWTOrKeyAuth(a.apiProfile))
	router.POST("/api/profile/regenerate-key", a.JWTOrKeyAuth(a.apiRegenerateKey))
	router.DELETE("/api/profile", a.JWTOrKeyAuth(a.apiDeleteAccount))
	router.GET("/api/domains", a.JWTOrKeyAuth(a.apiGetDomains))
	router.POST("/api/domains", a.JWTOrKeyAuth(a.apiAddDomain))
	router.DELETE("/api/domains/:domain", a.JWTOrKeyAuth(a.apiRemoveDomain))
	router.GET("/api/records", a.JWTOrKeyAuth(a.apiGetRecords))
	router.GET("/api/keys", a.JWTOrKeyAuth(a.apiListKeys))
	router.POST("/api/keys", a.JWTOrKeyAuth(a.apiCreateKey))
	router.DELETE("/api/keys/:id", a.JWTOrKeyAuth(a.apiDeleteKey))

	// Admin endpoints — API key auth
	router.GET("/admin/users", a.AdminAuth(a.adminListUsers))
	router.POST("/admin/users", a.AdminAuth(a.adminCreateUser))
	router.DELETE("/admin/users/:id", a.AdminAuth(a.adminDeleteUser))
	router.GET("/admin/domains", a.AdminAuth(a.adminListDomains))
	router.POST("/admin/domains", a.AdminAuth(a.adminAddDomain))
	router.DELETE("/admin/domains/:domain", a.AdminAuth(a.adminRemoveDomain))
	router.GET("/admin/records", a.AdminAuth(a.adminListRecords))

	// SPA static files — serve from web/dist if it exists
	handler := a.withSPA(c.Handler(router))

	host := a.Config.API.IP + ":" + a.Config.API.Port

	cfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	switch a.Config.API.TLS {
	case httpreq.TLSProviderLetsEncrypt, httpreq.TLSProviderLetsEncryptStaging:
		magic := a.setupTLS(dnsservers)
		err = magic.ManageAsync(context.Background(), []string{a.Config.General.Domain})
		if err != nil {
			a.errChan <- err
			return
		}
		cfg.GetCertificate = magic.GetCertificate
		srv := &http.Server{
			Addr:         host,
			Handler:      handler,
			TLSConfig:    cfg,
			ErrorLog:     stderrorlog,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  120 * time.Second,
		}
		a.Logger.Infow("Listening HTTPS",
			"host", host,
			"domain", a.Config.General.Domain)
		err = srv.ListenAndServeTLS("", "")
	case httpreq.TLSProviderCert:
		srv := &http.Server{
			Addr:         host,
			Handler:      handler,
			TLSConfig:    cfg,
			ErrorLog:     stderrorlog,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  120 * time.Second,
		}
		a.Logger.Infow("Listening HTTPS",
			"host", host,
			"domain", a.Config.General.Domain)
		err = srv.ListenAndServeTLS(a.Config.API.TLSCertFullchain, a.Config.API.TLSCertPrivkey)
	default:
		a.Logger.Infow("Listening HTTP",
			"host", host)
		err = http.ListenAndServe(host, handler)
	}
	if err != nil {
		a.errChan <- err
	}
}

// withSPA wraps the router to serve the SPA for non-API, non-file paths.
// Priority: external web/dist (dev override) > embedded dist (production).
func (a *API) withSPA(router http.Handler) http.Handler {
	var spaFS fs.FS

	// Check for external web/dist first (development override)
	for _, dir := range []string{"web/dist", "./web/dist"} {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			spaFS = os.DirFS(dir)
			a.Logger.Infow("Serving SPA from filesystem", "dir", dir)
			break
		}
	}

	// Fall back to embedded dist
	if spaFS == nil {
		embedded, err := fs.Sub(web.DistFS, "dist")
		if err == nil {
			// Verify embedded FS has content
			if f, err := embedded.Open("index.html"); err == nil {
				f.Close()
				spaFS = embedded
				a.Logger.Info("Serving SPA from embedded files")
			}
		}
	}

	var fileServer http.Handler
	if spaFS != nil {
		fileServer = http.FileServer(http.FS(spaFS))
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// API, httpreq, and health endpoints go to the router
		if path == "/present" || path == "/cleanup" || path == "/health" || path == "/llms.txt" ||
			len(path) >= 4 && path[:4] == "/api" ||
			len(path) >= 6 && path[:6] == "/admin" {
			router.ServeHTTP(w, r)
			return
		}

		// Try to serve static file from SPA dist
		if spaFS != nil {
			name := strings.TrimPrefix(path, "/")
			if name == "" {
				name = "index.html"
			}
			if f, err := spaFS.Open(name); err == nil {
				f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
			// Fall back to index.html for SPA client-side routing
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}

		// No SPA available, fall through to router
		router.ServeHTTP(w, r)
	})
}

func (a *API) setupTLS(dnsservers []httpreq.NS) *certmagic.Config {
	provider := NewChallengeProvider(dnsservers)
	certmagic.Default.Logger = a.Logger.Desugar()
	storage := certmagic.FileStorage{Path: a.Config.API.ACMECacheDir}

	certmagic.DefaultACME.DNS01Solver = &provider
	certmagic.DefaultACME.Agreed = true
	certmagic.DefaultACME.Logger = a.Logger.Desugar()
	if a.Config.API.TLS == httpreq.TLSProviderLetsEncrypt {
		certmagic.DefaultACME.CA = certmagic.LetsEncryptProductionCA
	} else {
		certmagic.DefaultACME.CA = certmagic.LetsEncryptStagingCA
	}
	certmagic.DefaultACME.Email = a.Config.API.NotificationEmail

	magicConf := certmagic.Default
	magicConf.Logger = a.Logger.Desugar()
	magicConf.Storage = &storage
	magicConf.DefaultServerName = a.Config.General.Domain
	magicCache := certmagic.NewCache(certmagic.CacheOptions{
		GetConfigForCert: func(cert certmagic.Certificate) (*certmagic.Config, error) {
			return &magicConf, nil
		},
		Logger: a.Logger.Desugar(),
	})
	magic := certmagic.New(magicCache, magicConf)
	return magic
}
