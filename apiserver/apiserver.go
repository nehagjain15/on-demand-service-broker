// Copyright (C) 2016-Present Pivotal Software, Inc. All rights reserved.
// This program and the accompanying materials are made available under the terms of the under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions and limitations under the License.

package apiserver

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"code.cloudfoundry.org/lager"
	"github.com/gorilla/mux"
	"github.com/pivotal-cf/brokerapi"
	apiauth "github.com/pivotal-cf/brokerapi/auth"
	"github.com/pivotal-cf/on-demand-service-broker/config"
	"github.com/pivotal-cf/on-demand-service-broker/loggerfactory"
	"github.com/pivotal-cf/on-demand-service-broker/mgmtapi"
	"github.com/urfave/negroni"
)

//go:generate counterfeiter -o fakes/combined_broker.go . CombinedBroker
type CombinedBroker interface {
	mgmtapi.ManageableBroker
	brokerapi.ServiceBroker
}

func New(
	conf config.Config,
	broker CombinedBroker,
	componentName string,
	mgmtapiLoggerFactory *loggerfactory.LoggerFactory,
	serverLogger *log.Logger,
) *http.Server {

	brokerRouter := mux.NewRouter()
	mgmtapi.AttachRoutes(brokerRouter, broker, conf.ServiceCatalog, mgmtapiLoggerFactory)
	brokerapi.AttachRoutes(brokerRouter, broker, lager.NewLogger(componentName))
	authProtectedBrokerAPI := apiauth.
		NewWrapper(conf.Broker.Username, conf.Broker.Password).
		Wrap(brokerRouter)

	dateFormat := "2006/01/02 15:04:05.000000"
	logFormat := "Request {{.Method}} {{.Path}} Completed {{.Status}} in {{.Duration}} | Start Time: {{.StartTime}}"
	negroniLogger := negroni.NewLogger()
	negroniLogger.ALogger = serverLogger
	negroniLogger.SetFormat(logFormat)
	negroniLogger.SetDateFormat(dateFormat)

	server := negroni.New(
		negroni.NewRecovery(),
		negroniLogger,
		negroni.NewStatic(http.Dir("public")),
	)

	server.UseHandler(authProtectedBrokerAPI)
	return &http.Server{
		Addr:    fmt.Sprintf(":%d", conf.Broker.Port),
		Handler: server,
	}
}

func StartAndWait(conf config.Config, server *http.Server, logger *log.Logger, stopServer chan os.Signal) {
	stopped := make(chan struct{})
	signal.Notify(stopServer, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stopServer

		timeoutSecs := conf.Broker.ShutdownTimeoutSecs
		logger.Printf("Broker shutting down on signal (timeout %d secs)...\n", timeoutSecs)

		ctx, cancel := context.WithTimeout(
			context.Background(),
			time.Second*time.Duration(timeoutSecs),
		)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			logger.Printf("Error gracefully shutting down server: %v\n", err)
		} else {
			logger.Println("Server gracefully shut down")
		}

		close(stopped)
	}()
	logger.Println("Listening on", server.Addr)
	var err error
	if conf.HasTLS() {
		acceptableCipherSuites := []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		}
		tlsConfig := tls.Config{
			CipherSuites: acceptableCipherSuites,
			MinVersion:   tls.VersionTLS12,
		}
		server.TLSConfig = &tlsConfig
		err = server.ListenAndServeTLS(conf.Broker.TLS.CertFile, conf.Broker.TLS.KeyFile)
	} else {
		err = server.ListenAndServe()
	}
	if err != http.ErrServerClosed {
		logger.Fatalf("Error listening and serving: %v\n", err)
	}
	<-stopped
}
