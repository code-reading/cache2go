/*
 * Simple caching library with expiration capabilities
 *     Copyright (c) 2013, Christian Muehlhaeuser <muesli@gmail.com>
 *
 *   For license see LICENSE.txt
 */

package cache2go

import (
	"sync"
	"time"
)

// Structure of an item in the cache.
// Parameter data contains the user-set value in the cache.
// 缓存项结构
type CacheItem struct {
	sync.RWMutex

	// The item's key.
	key interface{}
	// The item's data.
	data interface{}
	// How long will the item live in the cache when not being accessed/kept alive.
	lifeSpan time.Duration

	// Creation timestamp.
	createdOn time.Time
	// Last access timestamp.
	accessedOn time.Time
	// How often the item was accessed.
	accessCount int64

	// Callback method triggered right before removing the item from the cache
	//删除item之前回调此函数
	aboutToExpire func(key interface{})
}

// Returns a newly created CacheItem.
// Parameter key is the item's cache-key.
// Parameter lifeSpan determines after which time period without an access the item
// will get removed from the cache.
// Parameter data is the item's value.
//创建一个新的换成Item
func CreateCacheItem(key interface{}, lifeSpan time.Duration, data interface{}) CacheItem {
	t := time.Now()
	return CacheItem{
		key:           key,
		lifeSpan:      lifeSpan,
		createdOn:     t,
		accessedOn:    t,
		accessCount:   0,
		aboutToExpire: nil,
		data:          data,
	}
}

// Mark item to be kept for another expireDuration period.
//更新item访问时间和访问次数;
func (item *CacheItem) KeepAlive() {
	item.Lock()
	defer item.Unlock()
	item.accessedOn = time.Now()
	item.accessCount++
}

// Returns this item's expiration duration.
//返回item的生命周期;
func (item *CacheItem) LifeSpan() time.Duration {
	// immutable
	return item.lifeSpan
}

// Returns when this item was last accessed.
// 返回上次访问时间;
func (item *CacheItem) AccessedOn() time.Time {
	item.RLock()
	defer item.RUnlock()
	return item.accessedOn
}

// Returns when this item was added to the cache.
func (item *CacheItem) CreatedOn() time.Time {
	// immutable
	return item.createdOn
}

// Returns how often this item has been accessed.
//更新访问次数, 因为访问次数每次访问都会被修改, 所以返回之前先用读锁锁住;
func (item *CacheItem) AccessCount() int64 {
	item.RLock()
	defer item.RUnlock()
	return item.accessCount
}

// Returns the key of this cached item.
//返回item的key, 因为key在创建时指定, 此外一直不变 所以返回时不需要用锁;
func (item *CacheItem) Key() interface{} {
	// immutable
	return item.key
}

// Returns the value of this cached item.
func (item *CacheItem) Data() interface{} {
	// immutable
	return item.data
}

// Configures a callback, which will be called right before the item
// is about to be removed from the cache.
//设置回调函数, 它将在即将从缓存中删除项之前调用;
func (item *CacheItem) SetAboutToExpireCallback(f func(interface{})) {
	item.Lock()
	defer item.Unlock()
	item.aboutToExpire = f
}
