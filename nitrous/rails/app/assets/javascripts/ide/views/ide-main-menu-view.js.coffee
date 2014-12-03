menu = (name, items) -> App.Menu.create(name: name, items: items)
menuItem = (name, attrs={}) ->
  attrs.name = name
  App.MenuItem.createWithMixins(attrs)
separator = App.MenuItem.create(separator: true)

previewMenu = menu 'Preview',
  [ menuItem 'Port 3000 (SSL)',
      boxBinding: Em.Binding.oneWay('App.router.boxIdeController.content')
      target: '_blank'
      hide: (->
        !@get('box.region.ssl_enabled') # FIXME Use Em.Binding.not() when we upgrade Ember.
      ).property 'box.region.ssl_enabled'
      url: (->
        "https://#{@get 'box.boxHost'}"
      ).property 'box.boxHost'
  ].concat _.map([3000, 4000, 8080, 8888], (port) ->
    menuItem "Port #{port}",
      boxHostBinding: Em.Binding.oneWay('App.router.boxIdeController.content.boxHost')
      target: '_blank'
      url: (->
        if boxHost = @get 'boxHost'
          if port == 3000
            "http://#{boxHost}"
          else
            "http://#{boxHost}:#{port}"
      ).property 'boxHost').concat [
    separator
    menuItem 'Set Password...',
      disabledBinding: Em.Binding.oneWay('App.router.boxIdeController.content.notOwnBox')
      callback: (controller) ->
        controller.manageBasicAuth()
    separator
    menuItem 'Preview Help',
      url: 'http://help.nitrous.io/categories/previewing/'
      target: '_blank'
  ]

helpMenu = menu 'Help', [
  menuItem 'Help Center',
    url: 'http://help.nitrous.io/'
    target: '_blank'
  menuItem 'Web IDE Overview',
    url: 'http://help.nitrous.io/ide-general'
    target: '_blank'
  menuItem 'Frequently Asked Questions',
    url: 'http://help.nitrous.io/categories/faq'
    target: '_blank'
  separator
  menuItem 'Blog',
    url: 'http://blog.nitrous.io/'
    target: '_blank'
  menuItem 'Updates',
    url: 'http://updates.nitrous.io/'
    target: '_blank'
  separator
  menuItem 'Report an Issue...',
    url: 'mailto:support@nitrous.io?subject=Nitrous.IO%20Beta%20Feedback'
    target: '_blank'
]

rulerMenu = menu 'Ruler', [
  menuItem 'None',
    callback: (controller) ->
      controller.viewSetRulerColumn null
    checked: (->
      !App.get('router.ideEditorController.rulerColumn')
    ).property 'App.router.ideEditorController.rulerColumn'
  separator
].concat _.map [
  78
  80
  100
  120
], (column) ->
  menuItem column,
    callback: (controller) ->
      controller.viewSetRulerColumn column
    checked: (->
      column == App.get('router.ideEditorController.rulerColumn')
    ).property 'App.router.ideEditorController.rulerColumn'

themesMenu = menu 'Themes', _.map [
  # NOTE: keep these menu items sorted alphabetically within dark/light categories
  { name: 'base16-dark', label: 'Base16 Dark' }
  { name: 'solarized dark', label: 'Solarized Dark' }
  { name: 'tomorrow-night-eighties', label: 'Tomorrow Night Eighties' }
  { name: 'vibrant-ink', label: 'Vibrant Ink' }
  { name: 'divider', label: 'separator' }
  { name: 'base16-light', label: 'Base16 Light' }
  { name: 'paraiso-light', label: 'Paraiso Light' }
  { name: 'solarized light', label: 'Solarized Light' }
  { name: '3024-day', label: '3024 Day' }
], (theme) ->
  if theme.label == 'separator'
    separator
  else
    menuItem theme.label,
      callback: (controller) ->
        App.util.mixpanel "IDE Theme Change", name: theme.name
        App.util.mixpanelUpdateUser "IDE Theme": theme.name
        controller.viewSelectTheme theme.name
      checked: (->
        if currentTheme = App.get('router.ideEditorController.theme')
          currentTheme == theme.name
      ).property 'App.router.ideEditorController.theme'

App.IdeMainMenuView = Em.View.extend
  templateName: 'templates/box/ide/main-menu'
  elementId: 'ide_main_menu_view'
  heightBinding: 'App.router.ideLayoutController.mainMenuViewHeight'
  hideMainMenu: false

  noSecondaryMenuBinding: Em.Binding.oneWay 'App.isChromeWebView'

  menu: [
    menu 'Nitrous', [
      menuItem 'About Nitrous', callback: ->
        $('#about_nitrousio').modal 'show'
    ]
    menu 'File', [
      menuItem 'New', macKey: 'shift+ctrl+n', pcKey: 'ctrl+alt+n', callback: (controller) ->
        controller.fileNew()
      menuItem 'Save',
        macKey: ['command+s', 'ctrl+s', 'shift+ctrl+s'], pcKey: ['ctrl+s', 'ctrl+alt+s']
        disabledBinding: Em.Binding.oneWay('App.router.ideEditorController.hasNoTab')
        callback: (controller) ->
          controller.fileSave()
      separator
      menuItem 'Refresh Tree',
        macKey: ['shift+ctrl+r'], pcKey: ['ctrl+alt+r']
        disabledBinding: Em.Binding.oneWay('App.router.ideSidebarController.sftpSocketNotReady')
        callback: (controller) ->
          controller.fileRefreshTree()
      separator
      menuItem 'Go to File...',
        macKey: ['shift+ctrl+p'], pcKey: ['shift+ctrl+p']
        callback: (controller) ->
          controller.fileFinder()
      separator
      menuItem 'Close',
        macKey: ['shift+ctrl+w', 'ctrl+w'], pcKey: 'ctrl+alt+w'
        disabledBinding: Em.Binding.oneWay('App.router.ideEditorController.hasNoTab')
        callback: (controller) ->
          controller.fileClose()
    ]
    menu 'Edit', [
      menuItem 'Find',
        macKey: ['command+f'], pcKey: ['ctrl+f']
        disabledBinding: Em.Binding.oneWay('App.router.ideEditorController.hasNoTab')
        callback: (controller) ->
          controller.editFind()
      menuItem 'Find Next',
        macKey: ['command+g'], pcKey: ['ctrl+g']
        disabledBinding: Em.Binding.oneWay('App.router.ideEditorController.hasNoTab')
        callback: (controller) ->
          controller.editFindNext()
      menuItem 'Select Next',
        macKey: ['command+d'], pcKey: ['ctrl+d']
        disabledBinding: Em.Binding.oneWay('App.router.ideEditorController.hasNoTab')
        callback: (controller) ->
          controller.editSelectNext()
      menuItem 'Find Previous',
        macKey: ['shift+command+g'], pcKey: ['shift+ctrl+g']
        disabledBinding: Em.Binding.oneWay('App.router.ideEditorController.hasNoTab')
        callback: (controller) ->
          controller.editFindPrevious()
      menuItem 'Find in Project...',
        macKey: ['shift+command+f'], pcKey: ['shift+ctrl+f']
        callback: (controller) ->
          controller.editFindInProject()
      separator
      menuItem 'Replace',
        macKey: ['command+option+f'], pcKey: ['shift+ctrl+h']
        disabledBinding: Em.Binding.oneWay('App.router.ideEditorController.hasNoTab')
        callback: (controller) ->
          controller.editReplace()
      menuItem 'Replace All',
        macKey: ['shift+command+option+f'], pcKey: ['shift+ctrl+r']
        disabledBinding: Em.Binding.oneWay('App.router.ideEditorController.hasNoTab')
        callback: (controller) ->
          controller.editReplaceAll()
      separator
      menuItem 'Fold',
        disabledBinding: Em.Binding.oneWay('App.router.ideEditorController.hasNoTab')
        callback: (controller) ->
          controller.editFoldCode()
      menuItem 'Unfold',
        disabledBinding: Em.Binding.oneWay('App.router.ideEditorController.hasNoTab')
        callback: (controller) ->
          controller.editUnfoldCode()
      separator
      menuItem 'Toggle Comment',
        macKey: ['command+/'], pcKey: ['ctrl+/']
        disabledBinding: Em.Binding.oneWay('App.router.ideEditorController.hasNoTab')
        callback: (controller) ->
          controller.editToggleComment()
      menuItem 'Split Selection into Lines',
        macKey: ['shift+ctrl+l'], pcKey: ['shift+ctrl+l']
        disabledBinding: Em.Binding.oneWay('App.router.ideEditorController.hasNoTab')
        callback: (controller) ->
          controller.editSplitSelectionIntoLines()
      separator
      menuItem 'Auto-close Brackets & Quotes',
        callback: (controller) ->
          controller.toggleAutoCloseBrackets()
          true
        checkedBinding: Em.Binding.oneWay('App.router.ideEditorController.autoCloseBrackets')
      separator
      menuItem 'Use Vim Key Bindings',
        callback: (controller) ->
          if @get('checked')
            controller.editSetKeybindings('default')
          else
            controller.editSetKeybindings('vim')
        checked: (->
          'vim' == App.get('router.ideEditorController.keybindings')
        ).property 'App.router.ideEditorController.keybindings'
    ]
    menu 'View', [
      menuItem 'Select Next Tab', macKey: 'shift+ctrl+]', pcKey: 'shift+ctrl+]', callback: (controller) ->
        controller.viewSelectNextTab()
      menuItem 'Select Previous Tab', macKey: 'shift+ctrl+[', pcKey: 'shift+ctrl+[', callback: (controller) ->
        controller.viewSelectPreviousTab()
      separator
      menuItem 'Focus Console', macKey: 'shift+ctrl+c', pcKey: 'ctrl+alt+c', callback: (controller) ->
        controller.viewFocusConsole()
      menuItem 'Toggle Full Screen Console', macKey: 'shift+ctrl+f', pcKey: 'ctrl+alt+f', callback: (controller) ->
        controller.viewToggleFullScreenConsole()
      menuItem 'Toggle Full Screen Editor', macKey: 'shift+ctrl+e', pcKey: 'ctrl+alt+e', callback: (controller) ->
        controller.viewToggleFullScreenEditor()
      separator
      menuItem 'Themes', submenu: themesMenu
      separator
      menuItem 'Show Trailing Whitespace',
        callback: (controller) ->
          controller.viewToggleTrailingSpace()
          true
        checkedBinding: Em.Binding.oneWay('App.router.ideEditorController.showTrailingSpace')
      menuItem 'Show Line Numbers',
        callback: (controller) ->
          controller.viewToggleLineNumbers()
          true
        checkedBinding: Em.Binding.oneWay('App.router.ideEditorController.showLineNumbers')
      menuItem 'Highlight Line',
        callback: (controller) ->
          controller.viewToggleHighlightLine()
          true
        checkedBinding: Em.Binding.oneWay('App.router.ideEditorController.highlightLine')
      menuItem 'Highlight Matching Brackets',
        callback: (controller) ->
          controller.viewToggleMatchBrackets()
          true
        checkedBinding: Em.Binding.oneWay('App.router.ideEditorController.matchBrackets')
      menuItem 'Wrap Long Lines',
        callback: (controller) ->
          controller.viewToggleWrapLines()
          true
        checkedBinding: Em.Binding.oneWay('App.router.ideEditorController.wrapLines')
      menuItem 'Ruler', submenu: rulerMenu
    ]
    menu 'Collaborate', [
      menuItem 'Manage Collaborators...',
        disabledBinding: Em.Binding.oneWay('App.router.boxIdeController.content.notOwnBox')
        callback: (controller) ->
          controller.collaborateManageCollaborators()
      separator
      menuItem 'Toggle Chat', callback: (controller) ->
        controller.collaborateToggleChat()
    ]
    menu 'Autoparts', [
      menuItem 'Manage Packages...',
        callback: (controller) ->
          controller.manageAutoparts()
      separator
      menuItem 'Autoparts Help',
        url: 'http://help.nitrous.io/autoparts/'
        target: '_blank'
    ]
    previewMenu
    helpMenu
  ]

  altMenu: [
    previewMenu,
    helpMenu
  ]

  didInsertElement: ->
    @_super()
    $(window).bind 'resize.ideMainMenuView', _.bind(@onResize, this)
    this.onResize()

  willDestroyElement: ->
    @_super()
    $(window).unbind 'resize.ideMainMenuView'

  onResize: ->
    @set 'height', @$().outerHeight()

  currentYear: (->
    new Date().getFullYear()
  ).property()
