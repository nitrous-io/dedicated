#= require polyfills
#= require webfonts
#= require md5
#= require pointer_events_polyfill
#= require jquery.autosize
#= require jquery.timeago-1.4.1
#= require async-0.2.5
#= require pubsub-1.5.2
#= require ui
#= require_self

$ ->
  PointerEventsPolyfill.initialize(selector: '.click-thru')
