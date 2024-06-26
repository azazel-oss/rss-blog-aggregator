package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"log"
	"net/http"
	"os"
	"rssblogaggregator/internal/database"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"

	_ "github.com/lib/pq"
)

type apiConfig struct {
	DB *database.Queries
}

type authedHandler func(http.ResponseWriter, *http.Request, database.User)

func main() {
	godotenv.Load()
	port := os.Getenv("PORT")

	db_url := os.Getenv("DB_URL")

	db, err := sql.Open("postgres", db_url)
	dbQueries := database.New(db)
	apiCfg := apiConfig{DB: dbQueries}
	if err != nil {
		log.Fatal(err.Error())
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go apiCfg.WorkerToProcessFeeds(ctx, 60*time.Second, 10)
	serveMux := http.NewServeMux()
	server := http.Server{
		Handler: serveMux,
		Addr:    ":" + port,
	}
	serveMux.HandleFunc("/v1/healthz", handleServerHealthCheck)
	serveMux.HandleFunc("/v1/err", handleServerError)
	serveMux.HandleFunc("POST /v1/users", apiCfg.createUsers)
	serveMux.HandleFunc("GET /v1/users", apiCfg.getUserByApiKey)
	serveMux.HandleFunc("GET /v1/feeds", apiCfg.getFeeds)
	serveMux.HandleFunc("POST /v1/feeds", apiCfg.middlewareAuth(apiCfg.createFeed))

	serveMux.HandleFunc("POST /v1/feed_follows", apiCfg.middlewareAuth(apiCfg.createFeedFollow))
	serveMux.HandleFunc("DELETE /v1/feed_follows/{feedFollowId}", apiCfg.deleteFeedFollow)
	serveMux.HandleFunc("GET /v1/feed_follows", apiCfg.middlewareAuth(apiCfg.getFeedsFollowForUser))

	serveMux.HandleFunc("GET /v1/posts", apiCfg.middlewareAuth(apiCfg.getPostsByUser))
	log.Println("Starting server on :8080")
	server.ListenAndServe()
}

func handleServerHealthCheck(w http.ResponseWriter, _ *http.Request) {
	ResponseWithJson(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleServerError(w http.ResponseWriter, _ *http.Request) {
	ResponseWithError(w, http.StatusInternalServerError, "Internal Server Error")
}

func (a apiConfig) getFeedsFollowForUser(w http.ResponseWriter, r *http.Request, u database.User) {
	feed_follows, err := a.DB.GetFeedFollowForUser(r.Context(), u.ID)
	if err != nil {
		ResponseWithError(w, http.StatusInternalServerError, "something went wrong in the database")
		return
	}
	ResponseWithJson(w, http.StatusOK, feed_follows)
}

func (a apiConfig) createFeedFollow(w http.ResponseWriter, r *http.Request, u database.User) {
	type RequestBody struct {
		FeedId string
	}

	bodyJson := RequestBody{}
	json.NewDecoder(r.Body).Decode(&bodyJson)

	fId, err := uuid.Parse(bodyJson.FeedId)
	if err != nil {
		ResponseWithError(w, http.StatusBadRequest, "the id you provided for feed is malformed")
		return
	}
	feedFollowId, err := uuid.NewV7()
	if err != nil {
		ResponseWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	feedFollow := database.CreateFeedFollowParams{
		ID:        feedFollowId,
		FeedID:    fId,
		UserID:    u.ID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	res, err := a.DB.CreateFeedFollow(r.Context(), feedFollow)
	if err != nil {
		ResponseWithError(w, http.StatusInternalServerError, "something went wrong in the database")
		return
	}
	ResponseWithJson(w, http.StatusOK, res)
}

func (a apiConfig) deleteFeedFollow(w http.ResponseWriter, r *http.Request) {
	feedFollowId := r.URL.Query().Get("feedFollowId")
	fId, err := uuid.Parse(feedFollowId)
	if err != nil {
		ResponseWithError(w, http.StatusBadRequest, "the feed follow id you provided is wrong")
		return
	}
	err = a.DB.DeletFeedFollow(r.Context(), fId)
	if err != nil {
		ResponseWithError(w, http.StatusInternalServerError, "something went wrong in the database")
		return
	}
	ResponseWithJson(w, http.StatusNoContent, nil)
}

func (a apiConfig) getFeeds(w http.ResponseWriter, r *http.Request) {
	feeds, err := a.DB.GetFeeds(r.Context())
	if err != nil {
		ResponseWithError(w, http.StatusInternalServerError, "something went wrong in the database")
		return
	}
	ResponseWithJson(w, http.StatusOK, feeds)
}

func (a apiConfig) getPostsByUser(w http.ResponseWriter, r *http.Request, u database.User) {
	var limit int
	limitQueryParam := r.URL.Query().Get("limit")
	limitInt, err := strconv.Atoi(limitQueryParam)
	if err != nil {
		limit = 10
	} else {
		limit = limitInt
	}

	posts, err := a.DB.GetPostsByUser(r.Context(), database.GetPostsByUserParams{
		UserID: u.ID,
		Limit:  int32(limit),
	})
	if err != nil {
		ResponseWithError(w, http.StatusInternalServerError, "something went wrong in the database")
		return
	}
	ResponseWithJson(w, http.StatusOK, posts)
}

func (a apiConfig) createUsers(w http.ResponseWriter, r *http.Request) {
	type RequestBody struct {
		Name string `json:"name"`
	}
	body := RequestBody{}
	json.NewDecoder(r.Body).Decode(&body)
	userId, _ := uuid.NewV7()
	user := database.CreateUserParams{
		Name:      body.Name,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		ID:        userId,
	}

	u, err := a.DB.CreateUser(r.Context(), user)
	if err != nil {
		ResponseWithError(w, http.StatusInternalServerError, "we couldn't create this user")
		return
	}
	ResponseWithJson(w, http.StatusOK, u)
}

func (a apiConfig) getUserByApiKey(w http.ResponseWriter, r *http.Request) {
	if len(strings.Split(r.Header.Get("Authorization"), " ")) != 2 && strings.Split(r.Header.Get("Authorization"), " ")[0] != "Apikey" {
		ResponseWithError(w, http.StatusBadRequest, "the Authorization key is malformed")
		return
	}
	apikey := strings.Split(r.Header.Get("Authorization"), " ")[1]
	u, err := a.DB.GetUserByApikey(r.Context(), apikey)
	if err != nil {
		ResponseWithError(w, http.StatusInternalServerError, "this user could not be fetched")
		return
	}
	ResponseWithJson(w, http.StatusOK, u)
}

func (a *apiConfig) createFeed(w http.ResponseWriter, r *http.Request, u database.User) {
	type RequestBody struct {
		Name string `json:"name"`
		Url  string `json:"url"`
	}
	type ResponsBody struct {
		Feed       database.Feed       `json:"feed"`
		FeedFollow database.FeedFollow `json:"feed_follow"`
	}
	bodyJson := RequestBody{}
	err := json.NewDecoder(r.Body).Decode(&bodyJson)
	if err != nil {
		ResponseWithError(w, http.StatusBadRequest, "please check your request body")
		return
	}
	feedId, err := uuid.NewV7()
	if err != nil {
		ResponseWithError(w, http.StatusInternalServerError, "uuid couldn't be created")
		return
	}

	feed := database.CreateFeedParams{
		Name:      bodyJson.Name,
		Url:       bodyJson.Url,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		ID:        feedId,
		UserID:    u.ID,
	}
	f, err := a.DB.CreateFeed(r.Context(), feed)
	if err != nil {
		ResponseWithError(w, http.StatusInternalServerError, "couldn't create the feed for this user")
		return
	}
	followFeedId, err := uuid.NewV7()
	if err != nil {
		ResponseWithError(w, http.StatusInternalServerError, "uuid couldn't be created")
		return
	}
	followFeed := database.CreateFeedFollowParams{
		ID:        followFeedId,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		UserID:    u.ID,
		FeedID:    feed.ID,
	}
	ff, err := a.DB.CreateFeedFollow(r.Context(), followFeed)
	if err != nil {
		ResponseWithError(w, http.StatusInternalServerError, "couldn't create the feed for this user")
		return
	}
	ResponseWithJson(w, http.StatusCreated, ResponsBody{
		Feed:       f,
		FeedFollow: ff,
	})
}

func (a *apiConfig) middlewareAuth(handler authedHandler) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Apikey" {
			ResponseWithError(w, http.StatusBadRequest, "the Authorization key is malformed")
			return
		}
		apikey := parts[1]
		u, err := a.DB.GetUserByApikey(r.Context(), apikey)
		if err != nil {
			ResponseWithError(w, http.StatusUnauthorized, "we were unable to find this user")
			return
		}

		handler(w, r, u)
	})
}

func ScrapeDataFromLiveFeedByUrl(url string) (Rss, error) {
	res, err := http.Get(url)
	if err != nil {
		return Rss{}, err
	}
	rss := Rss{}
	xml.NewDecoder(res.Body).Decode(&rss)
	return rss, nil
}

func (a apiConfig) WorkerToProcessFeeds(ctx context.Context, interval time.Duration, n int32) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("Feed fetcher worker stopped")
			return
		case <-ticker.C:
			feeds, err := a.DB.GetNextFeedsToFetch(ctx, n)
			if err != nil {
				log.Printf("Error fetching feeds: %v", err)
				continue
			}
			var wg sync.WaitGroup
			for _, feed := range feeds {
				wg.Add(1)
				go func(f database.Feed) {
					defer wg.Done()
					a.processFeed(ctx, f)
				}(feed)
			}
		}
	}
}

func (a apiConfig) processFeed(ctx context.Context, feed database.Feed) {
	_, err := a.DB.MarkFeedFetched(ctx, feed.ID)
	if err != nil {
		log.Printf("Couldn't mark feed %s fetched: %v", feed.Name, err)
		return
	}
	feedData, err := ScrapeDataFromLiveFeedByUrl(feed.Url)
	if err != nil {
		log.Printf("Couldn't collect feed %s: %v", feed.Name, err)
		return
	}
	for _, item := range feedData.Channel.Item {
		publishedAt := sql.NullTime{}
		if t, err := time.Parse(time.RFC1123Z, item.PubDate); err == nil {
			publishedAt = sql.NullTime{
				Time:  t,
				Valid: true,
			}
		}
		_, err = a.DB.CreatePost(context.Background(), database.CreatePostParams{
			ID:          uuid.New(),
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
			FeedID:      feed.ID,
			Title:       item.Title,
			Description: item.Description,
			Url:         item.Link,
			PublishedAt: publishedAt,
		})
		if err != nil {
			if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
				continue
			}
			log.Printf("Couldn't create post: %v", err)
			continue
		}
	}
	log.Printf("Feed %s collected, %v posts found", feed.Name, len(feedData.Channel.Item))
}
