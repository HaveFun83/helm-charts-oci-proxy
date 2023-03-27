// Copyright 2023 Google LLC All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"errors"
	"fmt"
	"k8s.io/utils/env"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/container-registry/helm-charts-oci-proxy/registry"
	"github.com/spf13/cobra"
)

func newCmdRegistry() *cobra.Command {
	cmd := &cobra.Command{
		Use: "registry",
	}
	cmd.AddCommand(newCmdServe())
	return cmd
}

func newCmdServe() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Serve an in-memory registry implementation",
		Long: `This sub-command serves an in-memory registry implementation on port :8080 (or $PORT)

The command blocks while the server accepts pushes and pulls.

Contents are only stored in memory, and when the process exits, pushed data is lost.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			l := log.New(os.Stdout, "proxy-", log.LstdFlags)

			port, err := env.GetInt("PORT", 9000)
			if err != nil {
				l.Fatalln(err)
			}

			debug, _ := env.GetBool("DEBUG", false)
			cacheTTLMin, _ := env.GetInt("CACHE_TTL_MIN", 15)
			useTLS, _ := env.GetBool("USE_TLS", false)

			listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
			if err != nil {
				l.Fatalln(err)
			}
			portI := listener.Addr().(*net.TCPAddr).Port
			s := &http.Server{
				ReadHeaderTimeout: 5 * time.Second, // prevent slowloris, quiet linter
				Handler: registry.New(ctx,
					registry.Debug(debug),
					registry.CacheTTLMin(cacheTTLMin),
					registry.Logger(l),
				),
			}

			errCh := make(chan error)
			go func() {
				if useTLS {
					l.Printf("HTTP over TLS serving on port %d", portI)
					errCh <- s.ServeTLS(listener, "registry.pem", "registry-key.pem")
				} else {
					l.Printf("HTTP on port %d", portI)
					errCh <- s.Serve(listener)
				}
			}()

			<-ctx.Done()
			l.Println("shutting down...")
			if err := s.Shutdown(ctx); err != nil {
				return err
			}
			if err := <-errCh; !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		},
	}
}
