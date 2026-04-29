package admin

import (
	"fmt"

	"github.com/v03413/bepusdt/app/model"
)

func validateChannel(channel *model.Channel) error {
	if channel == nil {
		return fmt.Errorf("通道为空")
	}

	return nil
}
