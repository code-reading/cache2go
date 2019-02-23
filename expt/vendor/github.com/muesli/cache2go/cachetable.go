/*
 * Simple caching library with expiration capabilities
 *     Copyright (c) 2013, Christian Muehlhaeuser <muesli@gmail.com>
 *
 *   For license see LICENSE.txt
 */

package cache2go

import (
	"log"
	"sort"
	"sync"
	"time"
)

// Structure of a table with items in the cache.
//缓存表结构
type CacheTable struct {
	sync.RWMutex

	// The table's name.
	name string
	// All cached items.
	items map[interface{}]*CacheItem

	// Timer responsible for triggering cleanup.
	//触发清理的定时器
	cleanupTimer *time.Timer
	// Current timer duration.
	//当前间隔定时器
	cleanupInterval time.Duration

	// The logger used for this table.
	//当前表的logger对象
	logger *log.Logger

	// Callback method triggered when trying to load a non-existing key.
	//当加载一个不存在的key时触发回调函数
	//设置数据加载源函数
	loadData func(key interface{}, args ...interface{}) *CacheItem
	// Callback method triggered when adding a new item to the cache.
	//当新增一个cache item时触发的回调函数
	addedItem func(item *CacheItem)
	// Callback method triggered before deleting an item from the cache.
	aboutToDeleteItem func(item *CacheItem)
}

// Returns how many items are currently stored in the cache.
//返回长度
func (table *CacheTable) Count() int {
	table.RLock()
	defer table.RUnlock()
	return len(table.items)
}

// foreach all items
//遍历所有的缓存项
func (table *CacheTable) Foreach(trans func(key interface{}, item *CacheItem)) {
	table.RLock()
	defer table.RUnlock()

	for k, v := range table.items {
		trans(k, v)
	}
}

// Configures a data-loader callback, which will be called when trying
// to access a non-existing key. The key and 0...n additional arguments
// are passed to the callback function.
//配置数据加载回调函数, 当读取一个不存在key时触发回调, 回调函数形参列表(key interface{}, ...interface{})
func (table *CacheTable) SetDataLoader(f func(interface{}, ...interface{}) *CacheItem) {
	table.Lock()
	defer table.Unlock()
	table.loadData = f
}

// Configures a callback, which will be called every time a new item
// is added to the cache.
//每次添加新item触发此回调函数
func (table *CacheTable) SetAddedItemCallback(f func(*CacheItem)) {
	table.Lock()
	defer table.Unlock()
	table.addedItem = f
}

// Configures a callback, which will be called every time an item
// is about to be removed from the cache.
//每次删除item时触发此删除回调函数
func (table *CacheTable) SetAboutToDeleteItemCallback(f func(*CacheItem)) {
	table.Lock()
	defer table.Unlock()
	table.aboutToDeleteItem = f
}

// Sets the logger to be used by this cache table.
// 设置日志对象
func (table *CacheTable) SetLogger(logger *log.Logger) {
	table.Lock()
	defer table.Unlock()
	table.logger = logger
}

// Expiration check loop, triggered by a self-adjusting timer.
//通过自适应定时器 循环检测过期Item
//检测逻辑
//1.当清除定时器不为空时, 先关闭清除定时器, 即先关闭上次定时任务;
//2.当清除时间间隔大于0时, 则下一次触发时间是隔一个间隔周期后, 触发日志记录;
//3.循环遍历缓存项, 删除过期缓存项, 找到最近下一次删除的过期时间间隔;
//4.更新过期时间间隔， 当这个时间间隔来临时再次触发过期时间检测;
func (table *CacheTable) expirationCheck() {
	table.Lock()
	if table.cleanupTimer != nil {
		table.cleanupTimer.Stop()
	}
	if table.cleanupInterval > 0 {
		table.log("Expiration check triggered after", table.cleanupInterval, "for table", table.name)
	} else {
		table.log("Expiration check installed for table", table.name)
	}

	// Cache value so we don't keep blocking the mutex.
	items := table.items
	table.Unlock()

	// To be more accurate with timers, we would need to update 'now' on every
	// loop iteration. Not sure it's really efficient though.
	now := time.Now()
	smallestDuration := 0 * time.Second
	for key, item := range items {
		// Cache values so we don't keep blocking the mutex.
		item.RLock()
		lifeSpan := item.lifeSpan
		accessedOn := item.accessedOn
		item.RUnlock()
		//未设置过期时间，则忽略
		if lifeSpan == 0 {
			continue
		}
		//距离上次访问时间大于其生命周期，则过期，删除当前key
		if now.Sub(accessedOn) >= lifeSpan {
			// Item has excessed its lifespan.
			table.Delete(key)
		} else {
			// Find the item chronologically closest to its end-of-lifespan.
			//找到所有item中距离其生命周期最近的间隔时间
			//当存在一个Item, 其生命周期时间减去上次访问时间的时间间隔小于当前记录的最小时间间隔, 则更新为当前记录的最小时间间隔;
			if smallestDuration == 0 || lifeSpan-now.Sub(accessedOn) < smallestDuration {
				smallestDuration = lifeSpan - now.Sub(accessedOn)
			}
		}
	}

	// Setup the interval for the next cleanup run.
	table.Lock()
	table.cleanupInterval = smallestDuration
	if smallestDuration > 0 {
		//time.AfterFunc 会在当前协程内调用func(go table.expirationCheck())方法
		table.cleanupTimer = time.AfterFunc(smallestDuration, func() {
			go table.expirationCheck()
		})
	}
	table.Unlock()
}

// Adds a key/value pair to the cache.
// Parameter key is the item's cache-key.
// Parameter lifeSpan determines after which time period without an access the item
// will get removed from the cache.
// Parameter data is the item's value.
//添加key/value对到缓存中;
//当过了一个lifeSpan 还没有被访问过, 则会把这个key从缓存中removed掉;
func (table *CacheTable) Add(key interface{}, lifeSpan time.Duration, data interface{}) *CacheItem {
	item := CreateCacheItem(key, lifeSpan, data)

	// Add item to cache.
	table.Lock()
	//触发添加日志;
	table.log("Adding item with key", key, "and lifespan of", lifeSpan, "to table", table.name)
	table.items[key] = &item

	// Cache values so we don't keep blocking the mutex.
	expDur := table.cleanupInterval
	addedItem := table.addedItem
	table.Unlock()

	// Trigger callback after adding an item to cache.
	//当设置了回调函数后, 则触发回调函数;
	if addedItem != nil {
		addedItem(&item)
	}

	// If we haven't set up any expiration check timer or found a more imminent item.
	//如果设置了生命周期, 并且表格清除检测时间间隔为0,或者生命周期小于清除间隔 则理解触发过期检测;
	if lifeSpan > 0 && (expDur == 0 || lifeSpan < expDur) {
		table.expirationCheck()
	}

	return &item
}

// Delete an item from the cache.
func (table *CacheTable) Delete(key interface{}) (*CacheItem, error) {
	table.RLock()
	r, ok := table.items[key]
	if !ok {
		table.RUnlock()
		return nil, ErrKeyNotFound
	}

	// Cache value so we don't keep blocking the mutex.
	aboutToDeleteItem := table.aboutToDeleteItem
	table.RUnlock()

	// Trigger callbacks before deleting an item from cache.
	//回调删除函数
	//table级别的回调函数
	if aboutToDeleteItem != nil {
		aboutToDeleteItem(r)
	}

	r.RLock()
	defer r.RUnlock()
	//item级别的回调函数
	if r.aboutToExpire != nil {
		r.aboutToExpire(key)
	}

	table.Lock()
	defer table.Unlock()
	table.log("Deleting item with key", key, "created on", r.createdOn, "and hit", r.accessCount, "times from table", table.name)
	//真正删除相应key的item
	delete(table.items, key)

	return r, nil
}

// Test whether an item exists in the cache. Unlike the Value method
// Exists neither tries to fetch data via the loadData callback nor
// does it keep the item alive in the cache.
// 检测缓存中是否存在名为key的item, 不存在返回false, 否则返回true
// Exists函数检测到key的item不存在时 不会触发loadData回调函数 ，当存在时也不会去更新其cache的上次访问时间;
func (table *CacheTable) Exists(key interface{}) bool {
	table.RLock()
	defer table.RUnlock()
	_, ok := table.items[key]

	return ok
}

// Test whether an item not found in the cache. Unlike the Exists method
// NotExistsAdd also add data if not found.
//检查在cache是否没有item， 与Exists不同的是, 当item不存在时, NotFoundAdd会添加这个key的item;
func (table *CacheTable) NotFoundAdd(key interface{}, lifeSpan time.Duration, data interface{}) bool {
	table.Lock()
    //当表中存在名为key的item 则直接返回false;
	if _, ok := table.items[key]; ok {
		table.Unlock()
		return false
	}

	item := CreateCacheItem(key, lifeSpan, data)
	table.log("Adding item with key", key, "and lifespan of", lifeSpan, "to table", table.name)
	table.items[key] = &item

	// Cache values so we don't keep blocking the mutex.
	expDur := table.cleanupInterval
	addedItem := table.addedItem
	table.Unlock()

	// Trigger callback after adding an item to cache.
	//触发添加回调;
	if addedItem != nil {
		addedItem(&item)
	}

	// If we haven't set up any expiration check timer or found a more imminent item.
	//触发过期检测;
	if lifeSpan > 0 && (expDur == 0 || lifeSpan < expDur) {
		table.expirationCheck()
	}
	return true
}

// Get an item from the cache and mark it to be kept alive. You can pass
// additional arguments to your DataLoader callback function.
//访问指定key, 并且更新其访问时间; 可以在触发DataLoader回调函数中传递相应的形参;
func (table *CacheTable) Value(key interface{}, args ...interface{}) (*CacheItem, error) {
	table.RLock()
	r, ok := table.items[key]
	loadData := table.loadData
	table.RUnlock()

	if ok {
		// Update access counter and timestamp.
		//如果访问的值存在, 则更新其访问次数及访问时间, 并返回;
		r.KeepAlive()
		return r, nil
	}

	// Item doesn't exist in cache. Try and fetch it with a data-loader.
	//当值不存在缓存中时, 尝试去加载数据;
	//当设置了数据加载源函数时, 则取加载数据;
	if loadData != nil {
		item := loadData(key, args...)
		//当加载成功时, 则更新到当前缓存中;
		if item != nil {
			table.Add(key, item.lifeSpan, item.data)
			return item, nil
		}
        //返回key不存在, 也不在加载数据源中;
		return nil, ErrKeyNotFoundOrLoadable
	}

    //返回key不存在;
	return nil, ErrKeyNotFound
}

// Delete all items from cache.
//删除表中所有的缓存项, 并且关闭表定时器;
func (table *CacheTable) Flush() {
	table.Lock()
	defer table.Unlock()

	table.log("Flushing table", table.name)

	table.items = make(map[interface{}]*CacheItem)
	table.cleanupInterval = 0
	if table.cleanupTimer != nil {
		table.cleanupTimer.Stop()
	}
}

//CacheItem对
type CacheItemPair struct {
	Key         interface{}
	AccessCount int64
}

// A slice of CacheIemPairs that implements sort. Interface to sort by AccessCount.
type CacheItemPairList []CacheItemPair

func (p CacheItemPairList) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p CacheItemPairList) Len() int           { return len(p) }
func (p CacheItemPairList) Less(i, j int) bool { return p[i].AccessCount > p[j].AccessCount }

//返回访问最多的前count个缓存项;
func (table *CacheTable) MostAccessed(count int64) []*CacheItem {
	table.RLock()
	defer table.RUnlock()

	//缓存项排序; 快排;
	p := make(CacheItemPairList, len(table.items))
	i := 0
	for k, v := range table.items {
		p[i] = CacheItemPair{k, v.accessCount}
		i++
	}
	sort.Sort(p)

	var r []*CacheItem
	c := int64(0)
	for _, v := range p {
		//当提取个数大于count 则结束;
		if c >= count {
			break
		}

		item, ok := table.items[v.Key]
		if ok {
			r = append(r, item)
		}
		c++
	}

	return r
}

// Internal logging method for convenience.
//方便表内部使用的日志方法;
func (table *CacheTable) log(v ...interface{}) {
	if table.logger == nil {
		return
	}

	table.logger.Println(v)
}
