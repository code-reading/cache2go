## cache2go 源码分析

cache2go是一个用golang实现的并发安全的具有过期检查及自动删除过期key的缓存库;

### data-flow
![data-flow-all](imgs/cache2go_global.png)

![data-flow](imgs/cache2go.png)
