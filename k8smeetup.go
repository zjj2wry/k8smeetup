package main

import (
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/context"

	flag "github.com/spf13/pflag"
	"golang.org/x/oauth2"

	"io"
	"text/tabwriter"

	"github.com/google/go-github/github"
)

const (
	// Organization ...
	Organization = "k8smeetup"
	// Repository ...
	Repository = "kubernetes.github.io"
)

var (
	organization string
	repository   string
	token        string
	output       string
)

func init() {
	flag.StringVar(&repository, "repository", Repository, "repository name")
	flag.StringVar(&organization, "organization", Organization, "organization name")
	flag.StringVarP(&output, "output", "o", "", "output type, support json")
	flag.StringVar(&token, "token", "", "Github api token for rate limiting. Background: https://developer.github.com/v3/#rate-limiting and create a token: https://github.com/settings/tokens")
}

func main() {
	flag.Parse()
	var tc *http.Client
	if len(token) > 0 {
		tc = oauth2.NewClient(
			oauth2.NoContext,
			oauth2.StaticTokenSource(
				&oauth2.Token{AccessToken: token}),
		)
	}
	client := github.NewClient(tc)
	ctx := context.Background()

	ghb := &Github{
		client,
	}
	users, err := ghb.GetOrgMembers(ctx, Organization)
	if err != nil {
		fmt.Printf("Error get %s origanization members: %v", organization, err)
		os.Exit(1)
	}
	contributes := []Contributes{}

	for _, user := range users {
		u := ghb.GetUserInfo(ctx, *user.ID)
		name := "<none>"
		if u.Name != nil {
			name = *u.Name
		}
		company := "<none>"
		if u.Company != nil {
			company = *u.Company
		}
		email := "<none>"
		if u.Email != nil {
			email = *u.Email
		}
		contri := Contributes{
			login:   *user.Login,
			name:    name,
			company: company,
			email:   email,
		}
		contri.reviews = ghb.GetReviewedNumbers(ctx, organization, repository, *user.Login)
		contributes = append(contributes, contri)
	}
	printContributes(contributes, output)
}

func printContributes(contributes []Contributes, output string) {
	sort.Sort(byReviewd(contributes))
	headers := []string{"Login", "Name", "Company", "Email", "Reviews"}
	w := GetNewTabWriter(os.Stdout)
	defer w.Flush()
	_, err := fmt.Fprintf(w, "%s\n", strings.Join(headers, "\t"))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	for _, contris := range contributes {
		if contris.reviews <= 0 {
			continue
		}
		_, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\n", contris.login, contris.name, contris.company, contris.email, contris.reviews)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}
}

// Contributes ...
type Contributes struct {
	login   string
	name    string
	company string
	email   string
	reviews int
}

type byReviewd []Contributes

func (a byReviewd) Len() int           { return len(a) }
func (a byReviewd) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byReviewd) Less(i, j int) bool { return a[i].reviews >= (a[j].reviews) }

// Github ...
type Github struct {
	client *github.Client
}

func (g *Github) sleepIfRateLimitExceeded(ctx context.Context) {
	rateLimit, _, err := g.client.RateLimits(ctx)
	if err != nil {
		fmt.Printf("Problem in getting rate limit information %v\n", err)
		return
	}

	if rateLimit.Search.Remaining == 1 {
		timeToSleep := rateLimit.Search.Reset.Sub(time.Now()) + time.Second
		time.Sleep(timeToSleep)
	}
}

// GetOrgMembers ...
func (g *Github) GetOrgMembers(ctx context.Context, org string) ([]*github.User, error) {
	g.sleepIfRateLimitExceeded(ctx)
	ops := &github.ListMembersOptions{
		Filter: "all",
		Role:   "all",
	}
	users, _, err := g.client.Organizations.ListMembers(ctx, org, ops)
	if err != nil {
		return nil, err
	}
	return users, nil
}

// GetReviewedNumbers ...
func (g *Github) GetReviewedNumbers(ctx context.Context, org, repo, author string) int {
	g.sleepIfRateLimitExceeded(ctx)
	allReviewedPullRequestsquery := "is:pr repo:" + org + "/" + repo + " reviewed-by:" + author + " -author:" + author
	opt := &github.SearchOptions{}

	reviewedPullRequestResults, _, err := g.client.Search.Issues(ctx, allReviewedPullRequestsquery, opt)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	return reviewedPullRequestResults.GetTotal()
}

// GetUserInfo ...
func (g *Github) GetUserInfo(ctx context.Context, id int) *github.User {
	g.sleepIfRateLimitExceeded(ctx)
	user, _, err := g.client.Users.GetByID(ctx, id)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	return user
}

// GetPullRequests ...
func (g *Github) GetPullRequests(ctx context.Context, org, repo, author string) int {
	g.sleepIfRateLimitExceeded(ctx)
	allPullRequestsquery := "is:pr repo:" + org + "/" + repo + " -author:" + author
	opt := &github.SearchOptions{}

	pullRequestResults, _, err := g.client.Search.Issues(ctx, allPullRequestsquery, opt)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	return pullRequestResults.GetTotal()
}

const (
	tabwriterMinWidth = 10
	tabwriterWidth    = 4
	tabwriterPadding  = 3
	tabwriterPadChar  = ' '
	tabwriterFlags    = 0
)

// GetNewTabWriter returns a tabwriter that translates tabbed columns in input into properly aligned text.
func GetNewTabWriter(output io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(output, tabwriterMinWidth, tabwriterWidth, tabwriterPadding, tabwriterPadChar, tabwriterFlags)
}
