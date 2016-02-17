pages.Features =
  init: ->
    pages.activePageName = 'Features'

    appVersion = window.navigator.appVersion
    if appVersion.indexOf("Win") != -1
      os = "windows";
    else if appVersion.indexOf("Mac") != -1
      os = "osx"
    else if appVersion.indexOf("Linux") != -1
      os = "linux"
    else
      os = "nitrogen"

    $nitrogenDownloadBtn = $('.nitrous-desktop .download-btns .download')
    $nitrogenDownloadBtn.addClass(os)
