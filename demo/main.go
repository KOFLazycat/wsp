package main

import (
	"fmt"
	"os"

	"github.com/simplejia/clog"
	"github.com/simplejia/lc"

	"net/http"

	_ "github.com/simplejia/wsp/demo/clog"
	"github.com/simplejia/wsp/demo/conf"
	_ "github.com/simplejia/wsp/demo/mysql"
	_ "github.com/simplejia/wsp/demo/redis"
)

func init() {
	lc.Init(1e5)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
}

func main() {
	clog.Info("main()")

	s := &http.Server{
		Addr: fmt.Sprintf("%s:%d", conf.C.App.Ip, conf.C.App.Port),
	}
	err := s.ListenAndServe()
	clog.Error("main() s.ListenAndServe %v", err)
	os.Exit(-1)
}
