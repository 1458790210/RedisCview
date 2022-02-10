package lib

import (
	"errors"
)

func GetAccessToken(appId, appSecret string) (ak AccessToken, err error) {
	if ak.Errcode != 0 {
		return ak, errors.New(ak.Errmsg)
	}

	return ak, nil
}
