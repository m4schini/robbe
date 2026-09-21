// SPDX-License-Identifier: TODO

package ports

import "net/url"

// RedactURL returns raw with the password in its userinfo replaced by
// "xxxxx", for use in logs, output and error messages. Values without a
// password, including scp-like forms such as git@host:path, are returned
// unchanged.
func RedactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	if _, has := u.User.Password(); !has {
		return raw
	}

	return u.Redacted()
}
