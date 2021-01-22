This app is utterly useless to you, trust me.

I originally had a bunch of small Python scripts that grabbed data from
different sources and sent them to my LED display, which simply displays stuff
sent to it via MQTT.  The slippery slope started when I added temperature
sensor data to this, and I eventually ended up with multiple software defined
radios in multiple locations.  This required a bit more effort, and brought
things like remote reboot and secure MQTT to all of the scripts.  In the end I
opted to just merge them all into a single app and write it in Go for the heck
of it.

The general idea was to have all of the above config based so I didn't need to
muck with code to make changes, and one of the hardest things to do in that
world was deal with pub/sub to multiple MQTT servers cleanly.  I think I
solved this, at least in the config files (the actual Go is a little sketchy,
but I did write it so this is to be expected).

Anyway, the code to handle multiple brokers lives in broker_mux.go.  It works
just like typical MQTT code in Go, except it expects the broker name to be
part of the topic.  All of the work is around making that work so the config
file can specify things like broker:topic to describe the topic to pub/sub.

The rest of the code is just pulling in data from various sources and
formatting it in the way my LED matrix code expects it to be formatted.  Most
of this is text, but it does also support 64x32 images in 24bit color.
