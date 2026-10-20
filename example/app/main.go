// Sample app for the gate demo: healthy only while its database is reachable.
package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

func main() {
	db := net.JoinHostPort(os.Getenv("DB_HOST"), os.Getenv("DB_PORT"))

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		conn, err := net.DialTimeout("tcp", db, 2*time.Second)
		if err != nil {
			http.Error(w, "database unreachable", http.StatusServiceUnavailable)
			return
		}
		conn.Close()
		fmt.Fprintln(w, "ok")
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "sovereign-gate demo app")
	})

	log.Println("listening on :8080, database", db)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
