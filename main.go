// A partner for golang webserver
// Created by simplejia [7/2016]
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/printer"
	"go/token"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	controllerPath string
	filterPath     string
	lowerFlag      bool
	buf            = new(bytes.Buffer)
)

type filter struct {
	ImportPath  string
	PackageName string
	FuncName    string
	Params      map[string]interface{}
}

type ca struct {
	ImportPath   string
	PackageName  string
	RelativePath string
	TypeName     string
	MethodName   string
	PreFilters   []*filter
	PostFilters  []*filter
}

func exit() {
	println(buf.String())
	println("Failed!")
	os.Exit(-1)
}

func prettyPrint(in interface{}) string {
	bs, err := json.MarshalIndent(in, ".", "  ")
	if err != nil {
		return ""
	}
	return string(bs)
}

func lower(str string) string {
	if len(str) == 0 {
		return ""
	}
	return strings.ToLower(str[0:1]) + str[1:]
}

func getImportPath(file string) string {
	dir := filepath.Dir(file)
	for _, path := range build.Default.SrcDirs() {
		path, _ = filepath.Abs(path)
		path, _ = filepath.EvalSymlinks(path)
		dir, _ = filepath.EvalSymlinks(dir)
		if strings.HasPrefix(dir, path) {
			return dir[len(path)+1:]
		}
	}
	fmt.Fprintf(buf, "get import path fail\n")
	exit()
	return ""
}

func getRelativePath(controllerPath, file string) string {
	pathP, _ := filepath.Abs(controllerPath)
	pathP, _ = filepath.EvalSymlinks(pathP)
	pathC := filepath.Dir(file)
	pathC, _ = filepath.EvalSymlinks(pathC)
	if strings.HasPrefix(pathC, pathP) {
		return pathC[len(pathP):]
	}
	return ""
}

func getFilters(doc string, es []*ca) (preFilters, postFilters []*filter) {
	if len(doc) == 0 {
		return
	}

	f1 := func(token string) ([]string, map[string]map[string]interface{}) {
		token = "[" + token + "]"
		arr := []interface{}{}
		err := json.Unmarshal([]byte(token), &arr)
		if err != nil {
			fmt.Fprintf(buf, "Filter annotation: %s, error: %v [must be json array]\n", doc, err)
			exit()
		}
		arrRet, mapRet := []string{}, map[string]map[string]interface{}{}
		for _, e := range arr {
			switch ev := e.(type) {
			case string:
				arrRet = append(arrRet, ev)
				mapRet[ev] = nil
			case map[string]interface{}:
				for k, v := range ev {
					vv := map[string]interface{}{}
					d, _ := json.Marshal(v)
					err := json.Unmarshal(d, &vv)
					if err != nil {
						fmt.Fprintf(buf, "Filter annotation: %s[%s], error: %v [must be json object]\n", doc, d, err)
						exit()
					}
					arrRet = append(arrRet, k)
					mapRet[k] = vv
					break
				}
			}
		}
		return arrRet, mapRet
	}

	f2 := func(token string) (filters []*filter) {
		token1 := token + "("
		pos1 := strings.Index(doc, token1)
		if pos1 == -1 {
			return nil
		}
		token2 := ")"
		pos2 := strings.Index(doc[pos1+len(token1):], token2)
		if pos2 == -1 {
			return nil
		}
		pos2 += pos1 + len(token1)
		partial := doc[pos1+len(token1) : pos2]
		arr, hmap := f1(partial)
		for _, name := range arr {
			params := hmap[name]
			if !func() bool {
				for _, e := range es {
					if e.MethodName == name {
						filters = append(filters, &filter{
							ImportPath:  e.ImportPath,
							PackageName: e.PackageName,
							FuncName:    e.MethodName,
							Params:      params,
						})
						return true
					}
				}
				return false
			}() {
				fmt.Fprintf(buf, "Filter config fail, name not right: %s", name)
				exit()
			}
		}
		return
	}

	preFilters, postFilters = f2("@prefilter"), f2("@postfilter")

	return
}

func parseGo4Controller(file string, es4Filter []*ca) (es []*ca, err error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
	if err != nil {
		return
	}
	for _, decl := range f.Decls {
		mdecl, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if !mdecl.Name.IsExported() {
			continue
		}
		str := new(bytes.Buffer)
		printer.Fprint(str, fset, mdecl)
		matches := regexp.MustCompile(`func \(.+? \*?(.+?)\) .+?\(.+? .+?\.ResponseWriter, .+? \*.+?.Request\) {(.|\n)*}`).FindStringSubmatch(str.String())
		if len(matches) == 0 {
			continue
		}
		preFilters, postFilters := getFilters(mdecl.Doc.Text(), es4Filter)
		es = append(es, &ca{
			ImportPath:   getImportPath(file),
			PackageName:  f.Name.String(),
			RelativePath: getRelativePath(controllerPath, file),
			TypeName:     matches[1],
			MethodName:   mdecl.Name.String(),
			PreFilters:   preFilters,
			PostFilters:  postFilters,
		})
	}
	return
}

func parseGo4Filter(file string) (es []*ca, err error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		return
	}
	for _, decl := range f.Decls {
		mdecl, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if !mdecl.Name.IsExported() {
			continue
		}
		str := new(bytes.Buffer)
		printer.Fprint(str, fset, mdecl)
		if matched, _ := regexp.MatchString(`func .+?\(.+? .+?\.ResponseWriter, .+? \*.+?.Request, .+? map\[string\]interface{}\) bool {(.|\n)*}`, str.String()); !matched {
			continue
		}
		es = append(es, &ca{
			ImportPath:  getImportPath(file),
			PackageName: f.Name.String(),
			MethodName:  mdecl.Name.String(),
		})
	}
	return
}

func getGoFiles(path string) (files []string, err error) {
	err = filepath.Walk(path, func(path string, info os.FileInfo, err error) (reterr error) {
		if err != nil {
			reterr = err
			return
		}
		if info.IsDir() {
			return
		}
		if filepath.Ext(path) != ".go" {
			return
		}
		if strings.HasSuffix(path, "_test.go") {
			return
		}
		absPath, err := filepath.Abs(path)
		if err != nil {
			reterr = err
			return
		}
		files = append(files, absPath)
		return
	})
	if err != nil {
		return
	}

	return
}

func gen(es []*ca) (err error) {
	s := ""
	s += "// generated by wsp, DO NOT EDIT.\n\n"
	s += "package main\n\n"
	s += "import \"net/http\"\n"
	s += "import \"time\"\n"
	added := map[string]bool{}
	for _, e := range es {
		if added[e.ImportPath] {
			continue
		}
		added[e.ImportPath] = true
		s += "import \"" + e.ImportPath + "\"\n"
		for _, f := range e.PreFilters {
			if added[f.ImportPath] {
				continue
			}
			added[f.ImportPath] = true
			s += "import \"" + f.ImportPath + "\"\n"
		}
		for _, f := range e.PostFilters {
			if added[f.ImportPath] {
				continue
			}
			added[f.ImportPath] = true
			s += "import \"" + f.ImportPath + "\"\n"
		}
	}
	f := func(filters []*filter, space string) (str string) {
		for _, filter := range filters {
			params := "map[string]interface{}{"
			a := []string{}
			for k, v := range filter.Params {
				switch vv := v.(type) {
				case bool:
					a = append(a, "\""+k+"\": "+strconv.FormatBool(vv))
				case string:
					a = append(a, "\""+k+"\": "+"\""+vv+"\"")
				case float64:
					a = append(a, "\""+k+"\": "+strconv.FormatFloat(vv, 'f', -1, 64))
				default:
					fmt.Fprintf(buf, "Filter annotation: %v[%v], not support type\n", filter.Params, vv)
					exit()
				}
			}
			a = append(a, "\"__T__\": t")
			a = append(a, "\"__C__\": c")
			a = append(a, "\"__E__\": e")
			params += strings.Join(a, ", ")
			params += "}"
			str += space + "if ok := " + filter.PackageName + "." + filter.FuncName + "(w, r, " + params + "); !ok {\n"
			str += space + "\treturn\n"
			str += space + "}\n"
		}
		return
	}
	s += "\n"
	s += "func init() {\n"
	for _, e := range es {
		s += "\thttp.HandleFunc(\""
		if lowerFlag {
			s += lower(e.RelativePath) + "/" + lower(e.TypeName) + "/" + lower(e.MethodName)
		} else {
			s += e.RelativePath + "/" + e.TypeName + "/" + e.MethodName
		}
		s += "\", func(w http.ResponseWriter, r *http.Request) {\n"
		s += "\t\tt := time.Now()\n"
		s += "\t\t_ = t\n"
		s += "\t\tvar e interface{}\n"
		s += "\t\tc := new(" + e.PackageName + "." + e.TypeName + ")\n"
		s += "\t\tdefer func() {\n"
		s += "\t\t\te = recover()\n"
		s += f(e.PostFilters, "\t\t\t")
		s += "\t\t}()\n"
		s += f(e.PreFilters, "\t\t")
		s += "\t\tc." + e.MethodName + "(w, r)\n"
		s += "\t})\n\n"
	}
	s += "}"
	ioutil.WriteFile("WSP.go", []byte(s), 0644)
	return
}

func main() {
	flag.StringVar(&controllerPath, "c", "controller", "Specify the directory of controller path")
	flag.StringVar(&filterPath, "f", "filter", "Specify the directory of filter path")
	flag.BoolVar(&lowerFlag, "l", false, "Controller/Action, lower first alpha")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "A partner for golang webserver\n")
		fmt.Fprintf(os.Stderr, "version: 1.7, Created by simplejia [7/2016]\n\n")
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	// process filter
	es4Filter := func() (ret []*ca) {
		files, err := getGoFiles(filterPath)
		if err != nil {
			// 允许filter文件夹不存在
			if os.IsNotExist(err) {
				return
			}
			fmt.Fprintln(buf, "Scan source file error", err)
			exit()
		}

		fmt.Fprintln(buf, "\nScan files(filter):")
		for pos, file := range files {
			fmt.Fprintln(buf, pos, file)
		}

		fmt.Fprintln(buf, "\nProcess files(filter):")
		for pos, file := range files {
			fmt.Fprintln(buf, pos, file)
			es, err := parseGo4Filter(file)
			if err != nil {
				fmt.Fprintln(buf, "Parse source file error", err)
				exit()
			}

			ret = append(ret, es...)
		}

		fmt.Fprintln(buf, "\nMethod(Func) list(filter):")
		fmt.Fprintln(buf, prettyPrint(ret))
		return
	}()

	// process controller
	es4Controller := func() (ret []*ca) {
		files, err := getGoFiles(controllerPath)
		if err != nil {
			fmt.Fprintln(buf, "Scan source file error", err)
			exit()
		}

		fmt.Fprintln(buf, "\nScan files(controller):")
		for pos, file := range files {
			fmt.Fprintln(buf, pos, file)
		}

		fmt.Fprintln(buf, "\nProcess files(controller):")
		for pos, file := range files {
			fmt.Fprintln(buf, pos, file)
			es, err := parseGo4Controller(file, es4Filter)
			if err != nil {
				fmt.Fprintln(buf, "Parse source file error", err)
				exit()
			}

			ret = append(ret, es...)
		}

		fmt.Fprintln(buf, "\nMethod list(controller):")
		fmt.Fprintln(buf, prettyPrint(ret))
		return
	}()

	gen(es4Controller)

	println("Success!")
	return
}
