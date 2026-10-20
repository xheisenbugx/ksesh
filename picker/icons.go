package picker

import (
	"github.com/xheisenbugx/ksesh/icon"
	"github.com/xheisenbugx/ksesh/model"
)

func iconColWidth(config model.Config) int {
	return icon.ColumnWidth(config)
}

func iconWidth(icn string) int {
	return icon.Width(icn)
}
