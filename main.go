package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	parser "github.com/MemeLabs/chat-parser"
	// "mvdan.cc/xurls/v2"
	"nhooyr.io/websocket"
)

const addr = "wss://chat2.strims.gg/ws"

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

	// rxRelaxed := xurls.Relaxed()
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

			y := content["data"].(string)
			p := parser.NewParser(parserCtx, parser.NewLexer(y))
			span := p.ParseMessage()

			entities := make(map[string][]parser.Node)
			processNode(span, entities)
			z, _ := json.Marshal(entities)
			fmt.Println(string(z))
		}
	}
}

func processNode(node parser.Node, entities map[string][]parser.Node) {
	switch i := node.(type) {
	case *parser.Span:
		if len(i.Nodes) > 0 {
			for _, p := range i.Nodes {
				processNode(p, entities)
			}
		}
		switch i.Type {
		case parser.SpanSpoiler:
			entities["spoiler"] = append(entities["spoiler"], node)
		case parser.SpanCode:
			entities["code"] = append(entities["code"], node)
		case parser.SpanGreentext:
			entities["greentext"] = append(entities["greentext"], node)
		case parser.SpanMessage:
		}
	case *parser.Emote:
		entities["emotes"] = append(entities["emotes"], node)
	case *parser.Nick:
		entities["nicks"] = append(entities["nicks"], node)
	}
}
