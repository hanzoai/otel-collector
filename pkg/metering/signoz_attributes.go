package metering

import "regexp"

var (
	ExcludeHanzoO11yWorkspaceResourceAttrs = regexp.MustCompile("^o11y.workspace.*")
)
