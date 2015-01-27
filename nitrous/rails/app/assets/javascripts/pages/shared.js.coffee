pages.Shared =
  init: ->
    $ ->
      $.getJSON "/api/v0/app_settings", (settings)->
        window.mixpanel.init settings.mixpanel_token
        if pages.activePage
          mixpanel.register_once {"First Time": "TRUE"}
          mixpanel.track "#{pages.activePage} Viewed"
          mixpanel.register {"First Time": "FALSE"}
