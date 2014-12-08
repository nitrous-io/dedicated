#= require jquery
#= require jquery_ujs
#= require bootstrap
#= require ./vendor/underscore
#= require ./vendor/modernizr-html5-2.6.2
#= require ./vendor/json2
#= require_self
#= require ./license
#= require ./vendor/placeholder_polyfill.jquery

$ ->
  window.setTimeout(->
    $alert = $('#flash_messages .alert')
    if $('span', $alert).text().length > 0
      $alert.addClass 'in'
  , 500)

  $(window).on 'message', (event) ->
    if event.originalEvent.data == "chromeWebView"
      window.App?.set? 'isChromeWebView', true
      $('#page-header, #header-bottom').hide()
