pages.Pricing =
  init: ->
    pages.activePageName = 'Pricing'

    window.setTimeout(->
      window.location.replace '/app/#/pricing'
    , 1)
