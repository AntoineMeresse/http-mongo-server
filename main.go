package main

import (
	"fmt"
	"net/http"
)

type serverContext struct {
	portNumber int
}

func (s *serverContext) rootHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "This is the main page. Port used: %d", s.portNumber)
}

func main() {
	fmt.Println("Init server")
	ctx := serverContext{portNumber: 8080}
	port := fmt.Sprintf(":%d", ctx.portNumber)

	http.HandleFunc("/", ctx.rootHandler)

	fmt.Println("Server is listening on port http://localhost" + port)
	if err := http.ListenAndServe(port, nil); err != nil {
		fmt.Println("Error while starting server: ", err)
	}
}
