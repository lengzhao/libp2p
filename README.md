# libp2p

network over kcp(default), support plugin

直接使用服务端口进行交互(基于UDP)。而不是普通的client-server方式。
本地只会占用一个端口，这样允许连接无线多个节点，不会受端口数量限制。
由于只用了一个端口，所以NAT后的端口是固定的。
新节点的发现就非常简单，由于端口不会改变，可以直接连接。
如果有NAT拦截问题，只需要通过已经连接的第三个点帮忙传递一个NatTraversal消息就可以。

支持插件化，可以方便按需增删插件，做到功能解耦、模块独立

基于开源库KCP(github.com/xtaci/kcp-go和smux)
