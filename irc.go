package main

import (
	"regexp"
	"strings"

	"github.com/sztanpet/obscommits/internal/analyzer"
	"github.com/sztanpet/obscommits/internal/config"
	"github.com/sztanpet/obscommits/internal/debug"
	"github.com/sztanpet/obscommits/internal/factoids"
	"github.com/sztanpet/obscommits/internal/persist"
	"github.com/sztanpet/sirc"
	"golang.org/x/net/context"
	"gopkg.in/sorcix/irc.v1"
)

var (
	adminState *persist.State
	admins     map[string]struct{}
	adminRE    = regexp.MustCompile(`^\.(addadmin|deladmin|raw)\s+(.*)$`)
)

func initIRC(ctx context.Context) context.Context {
	adminRE.Longest()

	var err error
	adminState, err = persist.New("admins.state", &map[string]struct{}{
		"melkor":                       struct{}{},
		"sztanpet.users.quakenet.org":  struct{}{},
		"R1CH.users.quakenet.org":      struct{}{},
		"Jim.users.quakenet.org":       struct{}{},
		"Warchamp7.users.quakenet.org": struct{}{},
		"hwd.users.quakenet.org":       struct{}{},
		"paibox.users.quakenet.org":    struct{}{},
		"ThoNohT.users.quakenet.org":   struct{}{},
		"dodgepong.users.quakenet.org": struct{}{},
		"Sapiens.users.quakenet.org":   struct{}{},
	})
	if err != nil {
		d.F(err.Error())
	}

	admins = *adminState.Get().(*map[string]struct{})

	tcfg := config.FromContext(ctx)
	sirc.DebuggingEnabled = tcfg.Debug.Debug
	cfg := sirc.Config{
		Addr:     tcfg.IRC.Addr,
		Nick:     tcfg.IRC.Nick,
		Password: tcfg.IRC.Password,
		RealName: "http://obscommits.sztanpet.net/",
	}
	c := sirc.Init(cfg, func(c *sirc.IConn, m *irc.Message) bool {
		return handleIRC(ctx, c, m)
	})

	return c.ToContext(ctx)
}

func handleIRC(ctx context.Context, c *sirc.IConn, m *irc.Message) bool {
	if m.Command == irc.RPL_WELCOME {
		cfg := config.FromContext(ctx).IRC
		for _, ch := range cfg.Channels {
			c.Write(&irc.Message{Command: irc.JOIN, Params: []string{ch}})
		}

		return false
	}

	if m.Command != irc.PRIVMSG {
		return false
	}

	if factoids.Handle(c, m) == true {
		return true
	}
	if analyzer.Handle(c, m) == true {
		return true
	}

	if m.Prefix != nil && len(m.Prefix.Host) > 0 {
		adminState.Lock()
		_, admin := admins[m.Prefix.Host]
		adminState.Unlock()
		if !admin {
			return true
		}
		if factoids.HandleAdmin(c, m) {
			return true
		}

		if handleAdmin(c, m) {
			return true
		}
	}

	return false
}

func handleAdmin(c *sirc.IConn, m *irc.Message) bool {
	matches := adminRE.FindStringSubmatch(m.Trailing)
	if len(matches) == 0 {
		return false
	}
	adminState.Lock()
	// lifo defer order
	defer adminState.Save()
	defer adminState.Unlock()

	host := strings.TrimSpace(matches[2])
	switch matches[1] {
	case "addadmin":
		admins[host] = struct{}{}
		c.Notice(m, "Added host successfully")
	case "deladmin":
		delete(admins, host)
		c.Notice(m, "Removed host successfully")
	case "raw":
		nm := irc.ParseMessage(matches[2])
		if nm == nil {
			c.Notice(m, "Could not parse, are you sure you know the irc protocol?")
		} else {
			go c.Write(nm)
		}
	}

	return true
}
