pages.Shared =
  init: ->
    $.getJSON "/api/v0/app_settings", (settings) ->
      Mp.init(settings.mixpanel_token)
      if pages.activePageName
        Mp.track("Viewed #{pages.activePageName}")
