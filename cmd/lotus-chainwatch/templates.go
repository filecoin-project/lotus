package main

import (
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	rice "github.com/GeertJohan/go.rice"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
)

type handler struct {
	api    api.FullNode
	st     *storage
	site   *rice.Box
	assets http.Handler

	templates map[string]*template.Template
}

func newHandler(api api.FullNode, st *storage) (*handler, error) {
	h := &handler{
		api:  api,
		st:   st,
		site: rice.MustFindBox("site"),

		templates: map[string]*template.Template{},
	}
	h.assets = http.FileServer(h.site.HTTPBox())

	funcs := template.FuncMap{
		"count":    h.count,
		"countCol": h.countCol,
		"sum":      h.sum,
		"netPower": h.netPower,
		"queryNum": h.queryNum,
		"sizeStr":  sizeStr,
		"strings":  h.strings,
		"qstr":     h.qstr,
		"qstrs":    h.qstrs,
		"messages": h.messages,

		"pageDown": pageDown,
		"parseInt": func(s string) (int, error) { i, e := strconv.ParseInt(s, 10, 64); return int(i), e },
		"substr":   func(i, j int, s string) string { return s[i:j] },
		"sub":      func(a, b int) int { return a - b }, // TODO: really not builtin?

		"param": func(string) string { return "" }, // replaced in request handler
	}

	base := template.New("")

	base.Funcs(funcs)

	return h, h.site.Walk("", func(path string, info os.FileInfo, err error) error {
		if filepath.Ext(path) != ".html" {
			return nil
		}
		if err != nil {
			return err
		}
		log.Info(path)

		h.templates["/"+path], err = base.New(path).Parse(h.site.MustString(path))
		return err
	})
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h, err := newHandler(h.api, h.st) // for faster dev
	if err != nil {
		log.Error(err)
		return
	}

	p := r.URL.Path
	if p == "/" {
		p = "/index.html"
	}

	t, ok := h.templates[p]
	if !ok {
		h.assets.ServeHTTP(w, r)
		return
	}

	t, err = t.Clone()
	if err != nil {
		log.Error(err)
		return
	}

	t.Funcs(map[string]interface{}{
		"param": r.FormValue,
	})

	if err := t.Execute(w, nil); err != nil {
		log.Errorf("%+v", err)
		return
	}

	log.Info(r.URL.Path)
}

// Template funcs

func (h *handler) count(table string, filters ...string) (int, error) {
	// explicitly not caring about sql injection too much, this doesn't take user input

	filts := ""
	if len(filters) > 0 {
		filts = " where "
		for _, filter := range filters {
			filts += filter + " and "
		}
		filts = filts[:len(filts)-5]
	}

	var c int
	err := h.st.db.QueryRow("select count(1) from " + table + filts).Scan(&c)
	if err != nil {
		return 0, err
	}

	return c, nil
}

func (h *handler) countCol(table string, col string, filters ...string) (int, error) {
	// explicitly not caring about sql injection too much, this doesn't take user input

	filts := ""
	if len(filters) > 0 {
		filts = " where "
		for _, filter := range filters {
			filts += filter + " and "
		}
		filts = filts[:len(filts)-5]
	}

	var c int
	err := h.st.db.QueryRow("select count(distinct " + col + ") from " + table + filts).Scan(&c)
	if err != nil {
		return 0, err
	}

	return c, nil
}

func (h *handler) sum(table string, col string) (types.BigInt, error) {
	return h.queryNum("select sum(cast(" + col + " as bigint)) from " + table)
}

func (h *handler) netPower(slashFilt string) (types.BigInt, error) {
	if slashFilt != "" {
		slashFilt = " where " + slashFilt
	}
	return h.queryNum(`select sum(power) from (select distinct on (addr) power, slashed_at from miner_heads
    inner join blocks b on miner_heads.stateroot = b.parentStateRoot
order by addr, height desc) as p` + slashFilt)
}

func (h *handler) queryNum(q string, p ...interface{}) (types.BigInt, error) {
	// explicitly not caring about sql injection too much, this doesn't take user input

	var c string
	err := h.st.db.QueryRow(q, p...).Scan(&c)
	if err != nil {
		log.Error("qnum ", q, p, err)
		return types.NewInt(0), err
	}

	i := types.NewInt(0)
	_, ok := i.SetString(c, 10)
	if !ok {
		return types.NewInt(0), xerrors.New("num parse error: " + c)
	}
	return i, nil
}

var units = []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB", "ZiB"}

func sizeStr(size types.BigInt) string {
	size = types.BigMul(size, types.NewInt(100))
	i := 0
	for types.BigCmp(size, types.NewInt(102400)) >= 0 && i < len(units)-1 {
		size = types.BigDiv(size, types.NewInt(1024))
		i++
	}
	return fmt.Sprintf("%s.%s %s", types.BigDiv(size, types.NewInt(100)), types.BigMod(size, types.NewInt(100)), units[i])
}

func (h *handler) strings(table string, col string, filter string, args ...interface{}) (out []string, err error) {
	if len(filter) > 0 {
		filter = " where " + filter
	}
	log.Info("strings qstr ", "select "+col+" from "+table+filter, args)
	rws, err := h.st.db.Query("select "+col+" from "+table+filter, args...)
	if err != nil {
		return nil, err
	}
	for rws.Next() {
		var r string
		if err := rws.Scan(&r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}

	return
}

func (h *handler) qstr(q string, p ...interface{}) (string, error) {
	// explicitly not caring about sql injection too much, this doesn't take user input

	r, err := h.qstrs(q, 1, p...)
	if err != nil {
		return "", err
	}
	return r[0], nil
}

func (h *handler) qstrs(q string, n int, p ...interface{}) ([]string, error) {
	// explicitly not caring about sql injection too much, this doesn't take user input

	c := make([]string, n)
	ia := make([]interface{}, n)
	for i := range c {
		ia[i] = &c[i]
	}
	err := h.st.db.QueryRow(q, p...).Scan(ia...)
	if err != nil {
		log.Error("qnum ", q, p, err)
		return nil, err
	}

	return c, nil
}

func (h *handler) messages(filter string, args ...interface{}) (out []types.Message, err error) {
	if len(filter) > 0 {
		filter = " where " + filter
	}

	log.Info("select * from messages " + filter)

	rws, err := h.st.db.Query("select * from messages "+filter, args...)
	if err != nil {
		return nil, err
	}
	for rws.Next() {
		var r types.Message
		var cs string

		if err := rws.Scan(
			&cs,
			&r.From,
			&r.To,
			&r.Nonce,
			&r.Value,
			&r.GasPrice,
			&r.GasLimit,
			&r.Method,
			&r.Params,
		); err != nil {
			return nil, err
		}

		c, err := cid.Parse(cs)
		if err != nil {
			return nil, err
		}
		if c != r.Cid() {
			log.Warn("msg cid doesn't match")
		}

		out = append(out, r)
	}

	return
}

func pageDown(base, n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = base - i
	}

	return out
}

var _ http.Handler = &handler{}
