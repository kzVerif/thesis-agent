package servicehost

import (
	"context"
	"ws-agent/internal/apppaths"
)

const Name = apppaths.Name
const DisplayName = "Thesis Agent (Development)"
const Description = "Managed university lab Agent development Windows Service"

type Runtime func(context.Context, func()) error
