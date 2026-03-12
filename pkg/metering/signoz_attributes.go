package metering

import "regexp"

var (
	ExcludeHanzo O11yWorkspaceResourceAttrs = regexp.MustCompile("^o11y.workspace.*")
)
