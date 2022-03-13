
example (or designed) usage
===
[potato-routing]

situation
===
* for certain network environment
* supposedly there are two links `A` and `X`
	* route certain IP set `ipA` to `A` and the rest to `X`
	* each link will provide it's own DNS service `upstreamA` and `upstreamX`
* or three links `A`, `B` and `X`
	* similarly IP set `ipA` and `ipB`
	* and DNS services `upstreamA`, `upstreamB` and `upstreamX`

basic concept (for 2-way diverge)
===
* first query `upstreamA`
* if answer matches `ipA`, use it
* else query from `upstreamX` instead

other solutions
===
* domain list based solution
	* like [felixonmars/dnsmasq-china-list]
	* these are usually poorly maintained
		* not to criticize the maintainer but the nature of such projects.
	* for false positive matches, answers come from `upstreamA` but not in `ipA`
		* or false negative matches, answers will come from `upstreamX` but in `ipA`
		* this usually results in poor performance since the widely usage of CDN
	* in comparison there are fairly accurate IP sets
		* [gaoyifan/china-operator-ip]
		* [misakaio/chnroutes2]
		* [17mon/china_ip_list]
* similar IP set based solutions do exist
	* [shadowsocks/chinadns]
		* no activity since 2015
	* [shawn1m/overture]
	* [yuefeng/smartDNS]
		* forked from overture
		* not to be confused with [pymumu/smartdns]

designing choices
===
diverge is meant to be an intermediate layer between [AdGuard Home] and public DNS servers
* since the GUI of AdGuard Home is quite useful,
but most probably they will not merge this particular feature.
	* and obviously we wouldn't want to implement that again.
* also no DNS cache since AdGuard Home already had one.
* diverge decision is cached.

details
===
* for type `PTR` queries, the decision strategy is obvious
* for type `A` queries, the decision strategy described in basic concept is used
* for other types, do a type `A` query to `upstreamA` first
* n-way diverge is handled by simply trying `A`, `B`, `C`, ... one by one, if all of them fails, then `X`
	* plan the priority order and IP sets carefully
* there is a blocked domain list for like `lan` and `home.arpa`
* also a [special IPv4 list][iana-ipv4-special]

to do
===
- [x] diverge decision cache
	- [x] TTL (by Redis)
	- [x] non-volatile (by Redis)
	- [x] diagnostic/query, via CHAOS
	- [ ] diagnostic/dump, via HTTP?
- [x] <del>3-way</del> n-way diverge
- [x] fallback <del>and retry</del>
- [x] concurrent query
- [ ] bogus NXDOMAIN (like in dnsmasq)
- [ ] DoT/DoH support
- [ ] port to Rust, or Deno?

dependency
===
* [miekg/dns]
* [Redigo]

[potato-routing]: https://github.com/Jimmy-Z/potato-routing
[miekg/dns]: https://github.com/miekg/dns
[Redigo]: https://github.com/gomodule/redigo
[AdGuard Home]: https://adguard.com/en/adguard-home/overview.html
[iana-ipv4-special]: https://www.iana.org/assignments/iana-ipv4-special-registry/iana-ipv4-special-registry.xhtml
[dnsmasq]: http://www.thekelleys.org.uk/dnsmasq/doc.html
[gaoyifan/china-operator-ip]: https://github.com/gaoyifan/china-operator-ip
[misakaio/chnroutes2]: https://github.com/misakaio/chnroutes2
[17mon/china_ip_list]: https://github.com/17mon/china_ip_list
[felixonmars/dnsmasq-china-list]: https://github.com/felixonmars/dnsmasq-china-list
[shadowsocks/chinadns]: https://github.com/shadowsocks/ChinaDNS
[shawn1m/overture]: https://github.com/shawn1m/overture
[pymumu/smartdns]: https://github.com/pymumu/smartdns
[yuefeng/smartDNS]: https://github.com/import-yuefeng/smartDNS
