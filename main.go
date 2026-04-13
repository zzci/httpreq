package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/zzci/httpdns/pkg/httpdns"
	"github.com/zzci/httpdns/pkg/api"
	"github.com/zzci/httpdns/pkg/database"
	"github.com/zzci/httpdns/pkg/nameserver"

	"go.uber.org/zap"
)

func main() {
	setUmask()
	configPtr := flag.String("c", "./data/config.cfg", "config file location")
	flag.Parse()
	// Read global config
	var err error
	var logger *zap.Logger
	config, err := httpdns.ReadConfig(*configPtr)
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}
	logger, err = httpdns.SetupLogging(config)
	if err != nil {
		fmt.Printf("Could not set up logging: %s\n", err)
		os.Exit(1)
	}
	defer logger.Sync() //nolint:all
	sugar := logger.Sugar()

	sugar.Infow("Using config file",
		"file", *configPtr)
	sugar.Info("Starting up")
	db, err := database.Init(&config, sugar)
	// Error channel for servers
	errChan := make(chan error, 1)
	api := api.Init(&config, db, sugar, errChan)
	dnsservers := nameserver.InitAndStart(&config, db, sugar, errChan)
	go api.Start(dnsservers)
	if err != nil {
		sugar.Error(err)
	}
	for {
		err = <-errChan
		if err != nil {
			sugar.Fatal(err)
		}
	}
}
