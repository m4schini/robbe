// SPDX-License-Identifier: TODO

// Package redact hides secrets in values destined for logs and output.
package redact

import "net/url"

// URL returns raw with the password in its userinfo replaced by
// "xxxxx", for use in logs, output and error messages. Values without a
// password, including scp-like forms such as git@host:path, are returned
// unchanged.
func URL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	if _, has := u.User.Password(); !has {
		return raw
	}

	return u.Redacted()
}
