package config

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/gorilla/mux"
	"github.com/invopop/jsonschema"

	"github.com/filecoin-project/lotus/cmd/curio/deps"
	"github.com/filecoin-project/lotus/curiosrc/web/api/apihelper"
	"github.com/filecoin-project/lotus/node/config"
)

type cfg struct {
	*deps.Deps
}

func Routes(r *mux.Router, deps *deps.Deps) {
	c := &cfg{deps}
	r.Methods("GET").Path("/schema").HandlerFunc(getSch)
	r.Methods("GET").Path("/layers").HandlerFunc(c.getLayers)
	r.Methods("GET").Path("/topo").HandlerFunc(c.topo)
	r.Methods("GET").Path("/layers/{layer}").HandlerFunc(c.getLayer)
	r.Methods("POST").Path("/layers/{layer}").HandlerFunc(c.setLayer)
}
func getSch(w http.ResponseWriter, r *http.Request) {
	sch := jsonschema.Reflect(config.CurioConfig{})

	// add comments
	for k, doc := range config.Doc {
		item, ok := sch.Properties.Get(strings.ToLower(k))
		if !ok {
			continue
		}
		for _, line := range doc {
			item, ok := item.Properties.Get(strings.ToLower(line.Name))
			if !ok {
				continue
			}
			item.Description = line.Comment
		}
	}

	apihelper.OrHTTPFail(w, json.NewEncoder(w).Encode(sch))
}

func (c *cfg) getLayers(w http.ResponseWriter, r *http.Request) {
	var layers []string
	apihelper.OrHTTPFail(w, c.DB.Select(context.Background(), &layers, `SELECT title FROM harmony_config ORDER BY title`))
	apihelper.OrHTTPFail(w, json.NewEncoder(w).Encode(layers))
}

func (c *cfg) getLayer(w http.ResponseWriter, r *http.Request) {
	var layer string
	apihelper.OrHTTPFail(w, c.DB.Select(context.Background(), &layer, `SELECT config FROM harmony_config WHERE title = $1`, mux.Vars(r)["layer"]))

	// Read the TOML into a struct
	configStruct := map[string]any{} // NOT lotusproviderconfig b/c we want to preserve unsets
	_, err := toml.Decode(layer, &configStruct)
	apihelper.OrHTTPFail(w, err)

	// Encode the struct as JSON
	jsonData, err := json.Marshal(configStruct)
	apihelper.OrHTTPFail(w, err)

	// Write the JSON response
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(jsonData)
	apihelper.OrHTTPFail(w, err)
}

func (c *cfg) setLayer(w http.ResponseWriter, r *http.Request) {
	layer := mux.Vars(r)["layer"]
	var configStruct map[string]any
	apihelper.OrHTTPFail(w, json.NewDecoder(r.Body).Decode(&configStruct))

	// Encode the struct as TOML
	var tomlData bytes.Buffer
	err := toml.NewEncoder(&tomlData).Encode(configStruct)
	apihelper.OrHTTPFail(w, err)

	// Write the TOML to the database
	_, err = c.DB.Exec(context.Background(), `INSERT INTO harmony_config (title, config) VALUES ($1, $2) ON CONFLICT (title) DO UPDATE SET config = $2`, layer, tomlData.String())
	apihelper.OrHTTPFail(w, err)
}

func (c *cfg) topo(w http.ResponseWriter, r *http.Request) {
	var topology []struct {
		Server      string
		LastContact int64
		CPU         int
		GPU         int
		RAM         int
		//LayersCSV string `db:"layers"`
	}
	apihelper.OrHTTPFail(w, c.DB.Select(context.Background(), &topology, `
	SELECT 
	harmony_machines.host_and_port as server, last_contact
	cpu, gpu, ram
FROM harmony_machines 
ORDER BY server`,
	/*`
	SELECT
		COALESCE(harmony_machines.host_and_port, harmony_layers.host_and_port) as server,
		cpu, gpu, ram, layers
	FROM harmony_machines
	FULL JOIN harmony_layers
		ON harmony_machines.host_and_port = harmony_layers.host_and_port
	ORDER BY server`*/))
	w.Header().Set("Content-Type", "application/json")
	apihelper.OrHTTPFail(w, json.NewEncoder(w).Encode(topology))
}
