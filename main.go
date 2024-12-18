package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	irc "github.com/thoj/go-ircevent"
	"github.com/go-playground/webhooks/v6/github"
)

func main() {
	// Config data
	serverssl := os.Getenv("IRC_SERVER")
	channel := os.Getenv("IRC_CHANNEL")
	ircnick1 := os.Getenv("IRC_NICK")
	botmode := os.Getenv("IRC_BOTMODE")
	showforks := os.Getenv("IRC_SHOW_FORKS")
	showissues := os.Getenv("IRC_SHOW_ISSUES")

	// Internal state
	isupport := make(map[string]string)
	initialized := false

	initialize := func(e *irc.Event) {
		if ! initialized {
			if isupport["BOT"] != "" && botmode != "" {
				log.Printf("Enabling bot mode (+%s)\n", isupport["BOT"])
				e.Connection.Mode(ircnick1, "+" + isupport["BOT"])
			}
			e.Connection.Join(channel)
			initialized = true
		}
	}

	// Set up the connection
	conn := irc.IRC(ircnick1, ircnick1)
	if conn == nil {
		log.Println("conn is nil!  Did you set IRC_NICK?")
		return
	}

	// IRC auth config
	if _, use := os.LookupEnv("IRC_SASL"); use {
		conn.UseSASL = true
		conn.SASLLogin = os.Getenv("IRC_USER")
		conn.SASLPassword = os.Getenv("IRC_PASS")
		conn.SASLMech = "PLAIN"
	}

	// IRC startup
	conn.QuitMessage = "I've probably crashed..."
	conn.UseTLS = true
	// RPL_WELCOME
	conn.AddCallback("001", func(e *irc.Event) {
		log.Println("Connected to Server")
	})
	// RPL_ISUPPORT
	conn.AddCallback("005", func(e *irc.Event) {
		for _, tok := range e.Arguments {
			k, v, found := strings.Cut(tok, "=")
			if found {
				isupport[k] = v
			}
		}
	})
	// RPL_ENDOFMOTD
	conn.AddCallback("376", initialize)
	// ERR_NOMOTD
	conn.AddCallback("422", initialize)
	// RPL_ENDOFNAMES
	conn.AddCallback("366", func(e *irc.Event) {
		log.Println("Connected to Channel")
	})
	conn.AddCallback("PRIVMSG", func(e *irc.Event) {
		switch e.Message() {
		case ircnick1 + ": hello?":
			conn.Privmsgf(channel, "%s: go away, I'm busy", e.Nick)
		}
	})
	err := conn.Connect(serverssl)
	if err != nil {
		log.Println(err)
		return
	}

	// HTTP Setup
	hook, _ := github.New(github.Options.Secret(os.Getenv("WEBHOOK_SECRET")))
	http.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		payload, err := hook.Parse(r,
			github.PushEvent,
			github.PullRequestEvent,
			github.ForkEvent,
			github.IssuesEvent,
		)
		if err != nil {
			log.Println("Error parsing hook:", err)
			return
		}
		switch p := payload.(type) {
		case github.IssuesPayload:
			if p.Repository.Private || showissues == "" {
				return
			}
			action := ""
			switch p.Action {
			case "opened":
				action = "\x0303opened\x0f"
			case "closed":
				action = "\x0305closed\x0f"
			case "reopened":
				action = "\x0308reopened\x0f"
			default:
				return
			}
			conn.Noticef(channel, "[\x0302%s\x0f] \x0307%s\x0f %s issue \x0313#%d\x0f (\x0310%s\x0f): \x0314%s\x0f",
				p.Repository.Name, p.Sender.Login, action, p.Issue.Number, p.Issue.Title, p.Issue.HTMLURL)
		case github.PullRequestPayload:
			if p.Repository.Private {
				return
			}
			action := ""
			switch p.Action {
			case "opened":
				action = "\x0303opened\x0f"
			case "closed":
				action = "\x0305closed\x0f"
			case "reopened":
				action = "\x0308reopened\x0f"
			default:
				return
			}
			conn.Noticef(channel, "[\x0302%s\x0f] \x0307%s\x0f %s pull request \x0313#%d\x0f (\x0310%s\x0f): \x0314%s\x0f",
				p.Repository.Name, p.Sender.Login, action, p.Number, p.PullRequest.Title, p.PullRequest.HTMLURL)
		case github.PushPayload:
			if p.Repository.Private {
				return
			}
			if p.Ref != "refs/heads/master" {
				return
			}

			forced := ""
			if p.Forced {
				forced = "\x0304force-"
			}
			n_commits := len(p.Commits)
			commit_sfx := ""
			if n_commits != 1 {
				commit_sfx = "s"
			}
			before_sha := p.Before[0:8]
			after_sha := p.After[0:8]
			shortMsg := p.HeadCommit.Message
			idx := strings.Index(shortMsg, "\n")
			if idx != -1 {
				shortMsg = shortMsg[0:idx]
			}
			conn.Noticef(channel, "[\x0302%s\x0f] \x0307%s\x0f %spushed\x0f \x0308%d\x0f commit%s (%s -> %s, new HEAD: \x0310%s\x0f): \x0314%s\x0f",
				p.Repository.Name, p.Sender.Login, forced, n_commits, commit_sfx, before_sha, after_sha, shortMsg, p.Compare)
		case github.ForkPayload:
			if p.Repository.Private || showforks == "" {
				return
			}
			conn.Noticef(channel, "[\x0302%s\x0f] \x0307%s\x0f forked the repository to \x0307%s\x0f: \x0314%s\x0f",
				p.Repository.Name, p.Sender.Login, p.Forkee.FullName, p.Forkee.HTMLURL)
		}
	})

	// Shutdown handler setup
	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs

		log.Println(sig)
		done <- true
	}()

	// Startup Serving
	go conn.Loop()
	go http.ListenAndServe(":3000", nil)

	// Shut down
	<-done
	log.Println("exiting")
	conn.Privmsg(channel, "I'm going away now")
	conn.Quit()
}
