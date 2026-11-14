package metrics

const ratioApproximation = "; ratio of coarse averages approximates a count-weighted ratio only when intervals are nearly constant and both columns have matching null patterns; skipped collections or newly added columns can skew it; do not compute a ratio when an average is zero but its maximum is positive (rounding)"

var diskTimeDescriptions = []string{
	"milliseconds spent on reads per second, summed over overlapping requests (can exceed 1000; same material as iostat r_await); average completion time = read_time / read_ops (ms). Not computed when there are no operations. Not application response time" + ratioApproximation,
	"milliseconds spent on writes per second, summed over overlapping requests (can exceed 1000; same material as iostat w_await); average completion time = write_time / write_ops (ms). Not computed when there are no operations. Not application response time" + ratioApproximation,
}

var segmentDescriptions = []string{
	"segments sent per second including pure ACKs, excluding retransmissions (IPv4 + IPv6)",
	"retransmitted segments per second (IPv4 + IPv6); retransmission ratio = retrans_segs / out_segs (rough indicator, not packet loss rate)" + ratioApproximation,
}

var netQualityDescriptions = []string{
	"receive errors per second as reported by the NIC driver; virtual NICs may always report 0",
	"receive drops per second; includes frames discarded for having no protocol handler (e.g. unknown ethertype); this counter alone does not identify the cause or measure end-to-end packet loss",
	"transmit errors per second as reported by the NIC driver; virtual NICs may always report 0",
	"transmit drops per second as reported by the NIC driver; virtual NICs may always report 0",
}
