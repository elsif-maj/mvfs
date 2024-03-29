package app

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/elsif-maj/umbraSearch/db"
	"github.com/elsif-maj/umbraSearch/flows"
	"github.com/elsif-maj/umbraSearch/kvstore"
	"github.com/elsif-maj/umbraSearch/myEnv"
	"github.com/jackc/pgx/v5"
)

type Server struct {
	DBConn *pgx.Conn
	KVConn kvstore.KVStore
}

type apiFunc func(http.ResponseWriter, *http.Request) error

func (s *Server) GetDBConn() *pgx.Conn {
	return s.DBConn
}

func (s *Server) GetKVStore() kvstore.KVStore {
	return s.KVConn
}

func Setup() (*Server, error) {
	// Set environment variables if needed
	myEnv.SetEnv()

	// Connect to DB
	dbConn, err := db.ConnectDB()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	app := &Server{
		DBConn: dbConn,
	}

	// Connect to Key-Value store
	kvConn := kvstore.ConnectRedis()
	// error handling
	app.KVConn = kvConn

	return app, nil
}

func MakeAPIFunc(fn apiFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	return json.NewEncoder(w).Encode(v)
}

func (app *Server) HandleSnippets(w http.ResponseWriter, r *http.Request) error {
	snippets, err := db.GetAllSnippets(app.DBConn)
	if err != nil {
		return fmt.Errorf("Problem with db.GetAllSnippets: %v", err)
	}

	return writeJSON(w, http.StatusOK, snippets)
}

// Test:
// curl -X POST -H "Content-Type: application/json" -d '{"id":16}' http://localhost:3000/api/snippets/new
func (app *Server) HandleNewSnippet(w http.ResponseWriter, r *http.Request) error {
	if r.Method != "POST" {
		return writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
	}

	// extract all below to "HandleNewSnippetFlow"?
	var data map[string]interface{}
	err := json.NewDecoder(r.Body).Decode(&data)
	if err != nil {
		return fmt.Errorf("Problem with decoding request body: %v", err)
	}

	id, ok := data["id"].(float64)
	if !ok {
		return fmt.Errorf("Invalid or missing 'id' in request body JSON object")
	}

	snippetId := int(id)

	go func() {
		if err := flows.ProcessInputAsWords(app, snippetId); err != nil {
			log.Printf("Failed to process input: %v", err)
		}
	}()

	return writeJSON(w, http.StatusOK, map[string]string{"Success": fmt.Sprintf("The creation of snippet with DB primary key id: %d has been registered.", snippetId)})
}

func (app *Server) HandleIndexAllSnippets(w http.ResponseWriter, r *http.Request) error {
	snippets, err := db.GetAllSnippets(app.DBConn)
	if err != nil {
		return fmt.Errorf("Problem with db.GetAllSnippets: %v", err)
	}

	// extract all below to a 'flow'
	var allSnippetIds []int

	for i := 0; i < len(snippets); i++ {
		allSnippetIds = append(allSnippetIds, snippets[i].Id)
	}

	for _, v := range allSnippetIds {
		flows.ProcessInputAsWords(app, v)
	}

	return writeJSON(w, http.StatusOK, map[string]string{"Success": "Received request to (re)index all snippets"})
}

// Test:
// curl -X POST -H "Content-Type: application/json" -d '{"searchString":"return", "userId":"1308"}' http://localhost:3000/api/snippets/search
func (app *Server) HandleSearchString(w http.ResponseWriter, r *http.Request) error {
	if r.Method != "POST" {
		return writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
	}

	// extract all below to a flow?
	var data map[string]interface{}
	err := json.NewDecoder(r.Body).Decode(&data)
	if err != nil {
		return fmt.Errorf("Problem with decoding request body: %v", err)
	}

	searchString, ok := data["searchString"].(string)
	if !ok {
		return fmt.Errorf("Invalid or missing 'searchString' in request body JSON object")
	}
	userId, ok := data["userId"].(string)
	if !ok {
		return fmt.Errorf("Invalid or missing 'userId' in request body JSON object")
	}

	results, err := flows.Search(app, userId, searchString)
	if err != nil {
		return fmt.Errorf("failed to search the index: %w", err)
	}

	// resultsJson, _ := json.Marshal(results)

	return writeJSON(w, http.StatusOK, map[string][]string{"docIds": results})
}
