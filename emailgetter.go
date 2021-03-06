package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

var username = flag.String("username", "", "Username to execute the query")
var followers = flag.Bool("followers", false, "Get emails of follower users")
var following = flag.Bool("following", false, "Get emails of following users")
var noemails = flag.Bool("noemails", false, "Get the usernames instead of the emails")
var page = flag.Int("page", 1, "Page number for following and followers")

type EmailGetter struct {
	Addresses  []string
	RateLimit  bool
	OnlyUsers  bool
	PageNumber int
}

func (getter *EmailGetter) RetrieveEmail(wg *sync.WaitGroup, username string) {
	defer wg.Done()

	if getter.OnlyUsers {
		fmt.Println(username)
		return
	}

	/* Try to get it from the API */
	found := getter.ExtractFromAPI(username)

	if found == false {
		/* Try to get it from the profile page */
		found = getter.ExtractFromProfile(username)

		if found == false {
			/* Try to get it from the events endpoint */
			found = getter.ExtractFromActivity(username)
		}
	}
}

func (getter *EmailGetter) RetrieveFollowers(wg *sync.WaitGroup, username string) {
	getter.FriendEmails(wg, username, "followers")
}

func (getter *EmailGetter) RetrieveFollowing(wg *sync.WaitGroup, username string) {
	getter.FriendEmails(wg, username, "following")
}

func (getter *EmailGetter) FriendEmails(wg *sync.WaitGroup, username string, group string) {
	if getter.PageNumber > 1 {
		group += "?page=" + strconv.Itoa(getter.PageNumber)
	}

	content := getter.Request("https://github.com/" + username + "/" + group)
	pattern := regexp.MustCompile(`<img alt="@([^"]+)"`)
	friends := pattern.FindAllStringSubmatch(string(content), -1)

	for _, data := range friends {
		if data[1] != username {
			wg.Add(1) /* Add more emails */
			go getter.RetrieveEmail(wg, data[1])
		}
	}
}

func (getter *EmailGetter) ExtractFromAPI(username string) bool {
	/* Skip if API is rate limited */
	if getter.RateLimit == true {
		return false
	}

	content := getter.Request("https://api.github.com/users/" + username)
	output := string(content) /* Convert to facilitate readability */

	if strings.Contains(output, "rate limit exceeded") {
		getter.RateLimit = true
		return false
	}

	pattern := regexp.MustCompile(`"email": "([^"]+)",`)
	data := pattern.FindStringSubmatch(output)

	if len(data) == 2 && data[1] != "" {
		return getter.AppendEmail(data[1])
	}

	return false
}

func (getter *EmailGetter) ExtractFromProfile(username string) bool {
	content := getter.Request("https://github.com/" + username)
	pattern := regexp.MustCompile(`"mailto:([^"]+)"`)
	data := pattern.FindStringSubmatch(string(content))

	if len(data) == 2 && data[1] != "" {
		var urlEncoded string = data[1]

		urlEncoded = strings.Replace(urlEncoded, ";", "", -1)
		urlEncoded = strings.Replace(urlEncoded, "&#x", "%", -1)

		if out, err := url.QueryUnescape(urlEncoded); err == nil {
			return getter.AppendEmail(string(out))
		}
	}

	return false
}

func (getter *EmailGetter) ExtractFromActivity(username string) bool {
	/* Skip if API is rate limited */
	if getter.RateLimit == true {
		return false
	}

	content := getter.Request("https://api.github.com/users/" + username + "/repos?type=owner&sort=updated")
	pattern := regexp.MustCompile(`"full_name": "([^"]+)",`)
	data := pattern.FindStringSubmatch(string(content))

	if len(data) == 2 && data[1] != "" {
		commits := getter.Request("https://api.github.com/repos/" + data[1] + "/commits")
		expression := regexp.MustCompile(`"email": "([^"]+)",`)
		matches := expression.FindAllStringSubmatch(string(commits), -1)

		for _, match := range matches {
			getter.AppendEmail(match[1])
		}

		return len(matches) > 0
	}

	return false
}

func (getter *EmailGetter) Request(url string) []byte {
	client := http.Client{}

	req, err := http.NewRequest("GET", url, nil)

	req.Header.Set("DNT", "1")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Accept-Language", "en-US,en;q=0.8")
	req.Header.Set("User-Agent", "Mozilla/5.0 (KHTML, like Gecko) Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")

	if err != nil {
		panic(err)
	}

	resp, err := client.Do(req)

	defer resp.Body.Close()

	// I understand that ioutil.ReadAll is bad.
	content, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		panic(err)
	}

	return content
}

func (getter *EmailGetter) AppendEmail(email string) bool {
	var isAlreadyAdded bool = false

	for _, item := range getter.Addresses {
		if item == email {
			isAlreadyAdded = true
			break
		}
	}

	if isAlreadyAdded == false {
		getter.Addresses = append(getter.Addresses, email)
	}

	return true
}

func (getter *EmailGetter) PrintEmails() {
	for _, email := range getter.Addresses {
		fmt.Println(email)
	}
}

func main() {
	flag.Usage = func() {
		fmt.Println("E-Mail Getter")
		fmt.Println("  http://cixtor.com/")
		fmt.Println("  https://github.com/cixtor/emailgetter")
		fmt.Println("  https://en.wikipedia.org/wiki/Email_address_harvesting")
		fmt.Println("Usage:")
		flag.PrintDefaults()
	}

	flag.Parse()

	if *username == "" {
		fmt.Println("Missing username to query")
		flag.Usage()
	}

	var wg sync.WaitGroup
	var getter EmailGetter

	getter.PageNumber = *page

	if *noemails {
		getter.OnlyUsers = true
	}

	wg.Add(1) /* At least wait for one */
	go getter.RetrieveEmail(&wg, *username)

	if *following == true {
		getter.RetrieveFollowing(&wg, *username)
	} else if *followers == true {
		getter.RetrieveFollowers(&wg, *username)
	}

	wg.Wait()

	getter.PrintEmails()
}
