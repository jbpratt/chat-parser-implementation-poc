package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"

	parser "github.com/MemeLabs/chat-parser"
	"mvdan.cc/xurls/v2"
	"nhooyr.io/websocket"
)

const addr = "wss://chat.strims.gg/ws"

func main() {
	resp, err := http.Get("https://chat.strims.gg/emote-manifest.json")
	if err != nil {
		log.Fatalf("Failed to get emotes: %v", err)
	}
	defer resp.Body.Close()
	response := struct {
		Emotes []struct {
			Name string `json:"name"`
		} `json:"emotes"`
	}{}
	contents, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	if err = json.Unmarshal(contents, &response); err != nil {
		log.Fatal(err)
	}

	var emotes []string
	for _, emote := range response.Emotes {
		emotes = append(emotes, emote.Name)
	}
	jwt := os.Getenv("STRIMS_TOKEN")
	if jwt == "" {
		log.Fatal(fmt.Errorf("no jwt provided"))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c, _, err := websocket.Dial(ctx, addr,
		&websocket.DialOptions{
			HTTPHeader: http.Header{
				"Cookie": []string{fmt.Sprintf("jwt=%s", jwt)},
			},
		})
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close(websocket.StatusInternalError, "connection closed")

	userResponse := struct {
		Users []struct {
			Nick string `json:"nick"`
		} `json:"users"`
	}{}
	_, data, err := c.Read(ctx)
	if err != nil {
		log.Fatalf("failed to read from conn: %v", err)
	}

	x := "{" + strings.SplitN(string(data), "{", 2)[1]

	if err = json.Unmarshal([]byte(x), &userResponse); err != nil {
		log.Fatal(err)
	}
	var nicks []string
	for _, nick := range userResponse.Users {
		nicks = append(nicks, nick.Nick)
	}

	parserCtx := parser.NewParserContext(parser.ParserContextValues{
		Emotes:         emotes,
		Nicks:          nicks,
		Tags:           []string{"nsfw", "weeb", "nsfl", "loud"},
		EmoteModifiers: []string{"mirror", "flip", "rain", "snow", "rustle", "worth", "love", "spin", "wide", "lag", "hyper"},
	})

	rxRelaxed := xurls.Relaxed()
	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			log.Fatalf("failed to read from conn: %v", err)
		}

		msg := strings.SplitN(string(data), "{", 2)
		if strings.TrimSpace(msg[0]) == "MSG" {
			x := "{" + msg[1]
			var content map[string]interface{}
			if err = json.Unmarshal([]byte(x), &content); err != nil {
				log.Fatal(err)
			}

			msg := content["data"].(string)
			entities := extractEntities(
				parserCtx,
				rxRelaxed,
				msg,
			)

			z, _ := json.Marshal(entities)
			fmt.Printf("%q %+v\n", msg, string(z))
		}
	}
}

func extractEntities(parserCtx *parser.ParserContext, urls *regexp.Regexp, msg string) *Entities {
	e := &Entities{}
	addEntitiesFromSpan(e, parser.NewParser(parserCtx, parser.NewLexer(msg)).ParseMessage())

	for _, b := range urls.FindAllStringIndex(msg, -1) {
		e.Links = append(e.Links, &Link{
			URL:    msg[b[0]:b[1]],
			Bounds: [2]int{b[0], b[1]},
		})
	}

	return e
}

func addEntitiesFromSpan(e *Entities, span *parser.Span) {
	switch span.Type {
	case parser.SpanCode:
		e.Codes = append(e.Codes, &Code{
			Bounds: [2]int{span.Pos(), span.End()},
		})
	case parser.SpanSpoiler:
		e.Spoilers = append(e.Spoilers, &Spoiler{
			Bounds: [2]int{span.Pos(), span.End()},
		})
	case parser.SpanGreentext:
		e.Greentexts = append(e.Greentexts, &Greentext{
			Bounds: [2]int{span.Pos(), span.End()},
		})
	}

	for _, ni := range span.Nodes {
		switch n := ni.(type) {
		case *parser.Emote:
			e.Emotes = append(e.Emotes, &Emote{
				Name:   n.Name,
				Bounds: [2]int{n.Pos(), n.End()},
			})
		case *parser.Nick:
			e.Nicks = append(e.Nicks, &Nick{
				Nick:   n.Nick,
				Bounds: [2]int{n.Pos(), n.End()},
			})
		case *parser.Tag:
			e.Tags = append(e.Tags, &Tag{
				Name:   n.Name,
				Bounds: [2]int{n.Pos(), n.End()},
			})
		case *parser.Span:
			addEntitiesFromSpan(e, n)
		}
	}
}

type Link struct {
	URL    string `json:"url,omitempty"`
	Bounds [2]int `json:"bounds,omitempty"`
}

type Emote struct {
	Name   string `json:"name,omitempty"`
	Bounds [2]int `json:"bounds,omitempty"`
}

type Nick struct {
	Nick   string `json:"nick,omitempty"`
	Bounds [2]int `json:"bounds,omitempty"`
}

type Tag struct {
	Name   string `json:"name,omitempty"`
	Bounds [2]int `json:"bounds,omitempty"`
}

type Code struct {
	Bounds [2]int `json:"bounds,omitempty"`
}

type Spoiler struct {
	Bounds [2]int `json:"bounds,omitempty"`
}

type Greentext struct {
	Bounds [2]int `json:"bounds,omitempty"`
}

type Entities struct {
	Links      []*Link      `json:"links,omitempty"`
	Emotes     []*Emote     `json:"emotes,omitempty"`
	Nicks      []*Nick      `json:"nicks,omitempty"`
	Tags       []*Tag       `json:"tags,omitempty"`
	Codes      []*Code      `json:"codes,omitempty"`
	Spoilers   []*Spoiler   `json:"spoilers,omitempty"`
	Greentexts []*Greentext `json:"greentexts,omitempty"`
}
