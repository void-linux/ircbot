package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	irc "github.com/thoj/go-ircevent"
	"gopkg.in/go-playground/webhooks.v5/github"
)

func main() {
	// Config data
	serverssl := os.Getenv("IRC_SERVER")
	channel := os.Getenv("IRC_CHANNEL")
	ircnick1 := os.Getenv("IRC_NICK")

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
	conn.AddCallback("001", func(e *irc.Event) {
		log.Println("Connected to Server")
		conn.Join(channel)
	})
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
		)
		if err != nil {
			log.Println("Error parsing hook")
			return
		}
		switch p := payload.(type) {
		case github.PullRequestPayload:
			if p.Repository.Private {
				return
			}
			if p.Action != "opened" && p.Action != "closed" {
				return
			}
			conn.Noticef(channel, "%s %s #%d [%s] (%s)", p.Sender.Login, p.Action, p.Number, p.Repository.Name, p.PullRequest.Title)
		case github.PushPayload:
			if p.Repository.Private {
				return
			}
			if p.Ref != "refs/heads/master" {
				return
			}

			forced := ""
			if p.Forced {
				forced = "force-"
			}
			n_commits := len(p.Commits)
			commit_sfx := ""
			if n_commits > 1 {
				commit_sfx = "s"
			}
			before_sha := p.Before[0:8]
			after_sha := p.After[0:8]
			shortMsg := p.HeadCommit.Message
			idx := strings.Index(shortMsg, "\n")
			if idx != -1 {
				shortMsg = shortMsg[0:idx]
			}
			conn.Noticef(channel, "%s %spushed %d commit%s to %s (%s -> %s, new HEAD: %s)", p.Sender.Login, forced, n_commits,
				commit_sfx, p.Repository.Name, before_sha, after_sha, shortMsg)
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
