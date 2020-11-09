package version

import "fmt"

var (
	SHA string
	Tag string
)

func String() string {
	if Tag == "" {
		return SHA
	}

	return fmt.Sprintf("%s (%s)", Tag, SHA)
}
