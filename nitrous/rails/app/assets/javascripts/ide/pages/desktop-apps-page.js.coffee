#= require jquery
#= require ua-parser

window.App ||= {}

App.DesktopAppsPage =
  initialize: ->
    $(window).on 'hashchange', =>
      @showHidePlatform()

    if !(location.hash == '#windows' || location.hash == '#mac')
      os = new UAParser().getOS().name
      if os.indexOf('Mac') != -1 || os.indexOf('OS X') != -1
        location.hash = '#mac'
      else
        location.hash = '#windows'

    @showHidePlatform()

  showHidePlatform: ->
    if location.hash == '#mac'
      $('.windows').hide()
      $('.mac').show()
      $('html').addClass('html-mac')
    else
      $('.mac').hide()
      $('.windows').show()
      $('html').removeClass('html-mac')
