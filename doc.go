// +build doc

package main

import (
	"strings"
	"log"
)

func main() {
	App.CustomAppHelpTemplate = helpTpl
	err := App.Run([]string{"--help"})
	if err == nil {
		return
	}

	if strings.Contains(err.Error(), "Required flags") {
		return
	}

	log.Fatalf("failed to generate docs. %s", err)
}

const helpTpl = `## Configuration

Following configuration flags are supported by ` + "`tile38-cluster-manager`" + `.

Configuration can be provided either via command line flags or the environment variables mentioned against each flag.

{{range $index, $flag := .VisibleFlags}}{{if ne $flag.Name "help"}}
==` + "`--{{$flag.Name}}`" + ` ` + "`({{if $flag.Required}}required{{else}}optional{{end}}: {{if $flag.Value}}{{$flag.Value}}{{else}}\"\"{{end}})`, `env: " + `{{join $flag.EnvVars "` + "`" + `, ` + "`" + `"}}` + "`" + `==

:    {{$flag.Usage}}  

{{end}}{{end}}
`
