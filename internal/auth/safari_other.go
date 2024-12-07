//go:build !darwin

package auth

func detectSafari(debug bool) Browser {
	return Browser{Type: BrowserUnknown}
}

