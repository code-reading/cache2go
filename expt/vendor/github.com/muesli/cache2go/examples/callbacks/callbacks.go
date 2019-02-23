package main

import (
	"fmt"
	"time"

	"github.com/muesli/cache2go"
)

//三类回调;
//1.表级添加CacheItem回调;
//2.表级删除CacheItem回调;
//3.CacheItem过期回调;
func main() {
	cache := cache2go.Cache("myCache")

	// This callback will be triggered every time a new item
	// gets added to the cache.
	cache.SetAddedItemCallback(func(entry *cache2go.CacheItem) {
		fmt.Println("Added:", entry.Key(), entry.Data(), entry.CreatedOn())
	})
	// This callback will be triggered every time an item
	// is about to be removed from the cache.
	cache.SetAboutToDeleteItemCallback(func(entry *cache2go.CacheItem) {
		fmt.Println("Deleting:", entry.Key(), entry.Data(), entry.CreatedOn())
	})

	// Caching a new item will execute the AddedItem callback.
	cache.Add("someKey", 0, "This is a test!")

	// Let's retrieve the item from the cache
	res, err := cache.Value("someKey")
	if err == nil {
		fmt.Println("Found value in cache:", res.Data())
	} else {
		fmt.Println("Error retrieving value from cache:", err)
	}

	// Deleting the item will execute the AboutToDeleteItem callback.
	cache.Delete("someKey")

	// Caching a new item that expires in 3 seconds
	res = cache.Add("anotherKey", 3*time.Second, "This is another test")

	// This callback will be triggered when the item is about to expire
	res.SetAboutToExpireCallback(func(key interface{}) {
		fmt.Println("About to expire:", key.(string))
	})

	time.Sleep(5 * time.Second)
}