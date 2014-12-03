App.IdePartListingView = Em.View.extend
  templateName: 'templates/box/ide/part-listing'
  tagName: 'li'
  classNames: ['part_listing_view']

  content: null

  didClickInstall: ->
    @get('controller').install @get('content')

  didClickUninstall: ->
    @get('controller').uninstall @get('content')

  didClickStart: ->
    @get('controller').start @get('content')

  didClickStop: ->
    @get('controller').stop @get('content')
