package main

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mmxmb/quiet_hn/hn"
)

func main() {
	// parse flags
	var port, numStories int
	flag.IntVar(&port, "port", 3000, "the port to start the web server on")
	flag.IntVar(&numStories, "num_stories", 30, "the number of top stories to display")
	flag.Parse()

	tpl := template.Must(template.ParseFiles("./index.gohtml"))

	http.HandleFunc("/", handler(numStories, tpl))

	// Start the server
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}

// getStories gets all items with id in ids from HN API and returns a map from item.ID to item
func getStories(ids []int, client hn.Client) []item {
	itemChan := make(chan item, len(ids))

	// get HN items with ID in ids concurrently
	for _, id := range ids {
		go func(id int) {
			hnItem, err := client.GetItem(id)
			if err != nil {
				return
			}
			itemChan <- parseHNItem(hnItem)
		}(id)
	}

	ret := filterStories(itemChan, len(ids))
	close(itemChan)

	return ret
}

// filterStories consumes numItems items from itemChan and returns slice of items, each item is a story
func filterStories(itemChan <-chan item, numItems int) []item {
	ret := make([]item, 0, numItems)
	for i := 0; i < numItems; i++ {
		itm := <-itemChan
		if isStoryLink(itm) {
			ret = append(ret, itm)
		}
	}
	return ret
}

func getTopStories(ids []int, numStories int) []item {
	var client hn.Client
	idx := 0
	stories := make([]item, 0, numStories)

	// attempt getting more stories until we get sufficient number
	for len(stories) < numStories {
		numRemaining := numStories - len(stories)
		stories = append(stories, getStories(ids[idx:idx+numRemaining], client)...)
		idx += numRemaining
	}

	return sortStories(stories, ids) // get sorted slice of stories using ids
}

func handler(numStories int, tpl *template.Template) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		var client hn.Client
		ids, err := client.TopItems()
		if err != nil {
			http.Error(w, "Failed to load top stories", http.StatusInternalServerError)
			return
		}

		data := templateData{
			Stories: getTopStories(ids, numStories),
			Time:    time.Now().Sub(start),
		}
		err = tpl.Execute(w, data)
		if err != nil {
			http.Error(w, "Failed to process the template", http.StatusInternalServerError)
			return
		}
	})
}

// sortStories sorts stories so that the order of story.ID of each story
// is the same as order of each id in orderedIDs
func sortStories(stories []item, orderedIDs []int) []item {
	// create a map from item.ID to item
	m := make(map[int]item)
	for _, story := range stories {
		m[story.ID] = story
	}

	// orderedIDs determine the order of stories in the output slice (based on story.ID)
	ret := make([]item, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		itm, ok := m[id]
		if ok {
			ret = append(ret, itm)
		}
		if len(ret) >= len(stories) {
			break
		}
	}
	return ret
}

func isStoryLink(item item) bool {
	return item.Type == "story" && item.URL != ""
}

func parseHNItem(hnItem hn.Item) item {
	ret := item{Item: hnItem}
	u, err := url.Parse(ret.URL)
	if err == nil {
		ret.Host = strings.TrimPrefix(u.Hostname(), "www.")
	}
	return ret
}

// item is the same as the hn.Item, but adds the Host field
type item struct {
	hn.Item
	Host string
}

type templateData struct {
	Stories []item
	Time    time.Duration
}
