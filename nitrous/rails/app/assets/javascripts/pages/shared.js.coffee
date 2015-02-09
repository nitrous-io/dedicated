pages.Shared =
  init: ->
    $.getJSON "/api/v0/app_settings", (settings) ->
      mp.init(settings.mixpanel_token)
      if pages.activePageName
        mp.track("Viewed #{pages.activePageName}")
