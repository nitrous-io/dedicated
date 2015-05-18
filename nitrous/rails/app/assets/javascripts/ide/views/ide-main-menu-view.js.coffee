menu = (name, items) -> App.Menu.create(name: name, items: items)
menuItem = (name, attrs={}) ->
  attrs.name = name
  App.MenuItem.createWithMixins(attrs)
separator = App.MenuItem.create(separator: true)

rulerMenu = menu 'Ruler', [
  menuItem 'None',
    callback: (controller) ->
      controller.viewSetRulerColumn(0)
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
  { name: 'solarized dark', label: 'Solarized Dark' }
  { name: 'vibrant-ink', label: 'Vibrant Ink' }
  { name: 'monokai-dark-soda', label: 'Monokai Dark Soda'}
  { name: 'base16-dark', label: 'Base16 Dark' }
  { name: 'base16-eighties-dark', label: 'Base16 Eighties Dark' }
  { name: 'base16-ocean-dark', label: 'Base16 Ocean Dark' }
  { name: '---' }
  { name: 'solarized light', label: 'Solarized Light' }
  { name: 'github', label: 'GitHub' }
  # { name: '3024-day', label: '3024 Day' }
  # { name: 'paraiso-light', label: 'Paraiso Light' }
  { name: 'visual-studio', label: 'Visual Studio'}
  { name: 'base16-light', label: 'Base16 Light' }
  { name: 'base16-eighties-light', label: 'Base16 Eighties Light' }
  { name: 'base16-ocean-light', label: 'Base16 Ocean Light' }
], (theme) ->
  if theme.name == '---'
    separator
  else
    menuItem theme.label,
      callback: (controller) ->
        controller.windowSelectTheme theme.name
      checked: (->
        if currentTheme = App.get('router.ideEditorController.theme')
          currentTheme == theme.name
      ).property 'App.router.ideEditorController.theme'

App.IdeMainMenuView = Em.View.extend
  templateName: 'ide/main-menu'
  elementId: 'ide-main-menu-view'

  previewPortBinding: Em.Binding.oneWay 'App.container.previewPort'
  dashboardURLBinding: Em.Binding.oneWay 'App.dashboardURL'

  previewItems: (->
    baseItems = []
    App.get('container.previewablePorts').map (port)->
      txt = "Port #{port.containerPort}"
      if port.preview
        txt = "#{txt} (Default)"
      item = menuItem txt,
        url: "http://#{App.get 'container.preview_hostname'}:#{port.containerPort}"
        target: '_blank'
        track: Mp.EventIDEUserClickedPreviewLink

      if port.preview
        baseItems.unshift(item)
      else
        baseItems.push(item)

    if baseItems.length == 0
      baseItems.push(
        menuItem 'No ports available',
          disabled: true
      )

    baseItems.push(
      separator,
      menuItem 'Configure Ports',
        url: "#{@get('dashboardURL')}/ports"
        target: "_blank",
        track: Mp.EventIDEUserClickedConfigurePorts
      separator,
      menuItem 'Preview Help',
        url: 'http://docs.nitrous.io/v1.0/docs/preview'
        target: '_blank'
    )
    baseItems
  ).property 'App.container', 'App.container.previewablePorts.@each'

  helpMenuItems: (->
    baseItems = []
    if App.get('container.env.NITROUS_README_URL')
      baseItems.push(
        menuItem 'View README', callback: (controller) ->
          controller.helpNitrousReadme()
        separator
      )
    baseItems.push(
      menuItem 'Help Center',
        url: 'http://docs.nitrous.io/'
        target: '_blank'
      menuItem 'Web IDE Overview',
        url: 'http://docs.nitrous.io/v1.0/docs/web-ide-overview'
        target: '_blank'
      menuItem 'Frequently Asked Questions',
        url: 'http://docs.nitrous.io/v1.0/docs/billing--pricing-faq'
        target: '_blank'
      separator
      menuItem 'Blog',
        url: 'http://blog.nitrous.io/'
        target: '_blank'
      menuItem 'Updates',
        url: 'http://docs.nitrous.io/v1.0/blog'
        target: '_blank'
      separator
      menuItem 'Report an Issue...', callback: (controller) ->
        App.trigger 'open-feedback-dialog', 'Report an Issue', 'Your request has been submitted. Our support team will email you soon.'
    )
    baseItems
  ).property 'App.container', 'App.container.env', 'App.container.env.NITROUS_README_URL'

  menu: (->
    [
      menu 'N', [
        menuItem 'About Nitrous', callback: ->
          ui.showModal($('#about-nitrous'))
      ]
      menu 'File', [
        menuItem 'New', macKey: 'shift+ctrl+n', pcKey: 'ctrl+alt+n', callback: (controller) ->
          controller.fileNew()
        menuItem 'Save',
          macKey: ['command+s', 'ctrl+s', 'shift+ctrl+s'], pcKey: ['ctrl+s', 'ctrl+alt+s']
          disabled: (->
            App.get('router.ideEditorController.hasNoTab') || App.get('router.ideController.opsSocketNotReady')
          ).property 'App.router.ideEditorController.hasNoTab', 'App.router.ideController.opsSocketNotReady'
          callback: (controller) ->
            controller.fileSave()
        separator
        menuItem 'Refresh Tree',
          macKey: ['shift+ctrl+r'], pcKey: ['ctrl+alt+r']
          disabledBinding: Em.Binding.oneWay('App.router.ideSidebarController.opsSocketNotReady')
          callback: (controller) ->
            controller.fileRefreshTree()
        menuItem 'Go to File...',
          macKey: ['shift+ctrl+p'], pcKey: ['shift+ctrl+p']
          disabledBinding: Em.Binding.oneWay('App.router.ideController.opsSocketNotReady')
          callback: (controller) ->
            controller.fileGoToFile()
        separator
        menuItem 'Close',
          macKey: ['shift+ctrl+w', 'ctrl+w'], pcKey: 'ctrl+alt+w'
          callback: (controller) ->
            controller.fileClose()
      ]
      menu 'Edit', [
        menuItem 'Go to Line...',
          macKey: ['command+l'], pcKey: ['ctrl+l']
          disabledBinding: Em.Binding.oneWay('App.router.ideEditorController.hasNoTab')
          callback: (controller) ->
            controller.editGoToLine()
        separator
        menuItem 'Fold',
          disabledBinding: Em.Binding.oneWay('App.router.ideEditorController.isActiveTabNotAnEditor')
          callback: (controller) ->
            controller.editFoldCode()
        menuItem 'Unfold',
          disabledBinding: Em.Binding.oneWay('App.router.ideEditorController.isActiveTabNotAnEditor')
          callback: (controller) ->
            controller.editUnfoldCode()
        separator
        menuItem 'Toggle Comment',
          macKey: ['command+/'], pcKey: ['ctrl+/']
          disabledBinding: Em.Binding.oneWay('App.router.ideEditorController.isActiveTabNotAnEditor')
          callback: (controller) ->
            controller.editToggleComment()
        menuItem 'Split Selection into Lines',
          macKey: ['shift+ctrl+l'], pcKey: ['shift+ctrl+l']
          disabledBinding: Em.Binding.oneWay('App.router.ideEditorController.isActiveTabNotAnEditor')
          callback: (controller) ->
            controller.editSplitSelectionIntoLines()
        separator
        menuItem 'Auto-close Brackets & Quotes',
          callback: (controller) ->
            controller.toggleAutoCloseBrackets()
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
      menu 'Find', [
        menuItem 'Find',
          macKey: ['command+f'], pcKey: ['ctrl+f']
          disabledBinding: Em.Binding.oneWay('App.router.ideEditorController.isActiveTabNotAnEditor')
          callback: (controller) ->
            controller.findFind()
        separator
        menuItem 'Find Next',
          macKey: ['command+g'], pcKey: ['ctrl+g']
          disabledBinding: Em.Binding.oneWay('App.router.ideEditorController.isActiveTabNotAnEditor')
          callback: (controller) ->
            controller.findFindNext()
        menuItem 'Select Next',
          macKey: ['command+d'], pcKey: ['ctrl+d']
          disabledBinding: Em.Binding.oneWay('App.router.ideEditorController.isActiveTabNotAnEditor')
          callback: (controller) ->
            controller.findSelectNext()
        menuItem 'Find Previous',
          macKey: ['shift+command+g'], pcKey: ['shift+ctrl+g']
          disabledBinding: Em.Binding.oneWay('App.router.ideEditorController.isActiveTabNotAnEditor')
          callback: (controller) ->
            controller.findFindPrevious()
        separator
        menuItem 'Replace',
          macKey: ['command+option+f'], pcKey: ['shift+ctrl+h']
          disabledBinding: Em.Binding.oneWay('App.router.ideEditorController.isActiveTabNotAnEditor')
          callback: (controller) ->
            controller.findReplace()
        separator
        menuItem 'Find in Files...',
          macKey: ['shift+command+f'], pcKey: ['shift+ctrl+f']
          disabledBinding: Em.Binding.oneWay('App.router.ideController.opsSocketNotReady')
          disabled: true
          callback: (controller) ->
            controller.findFindInFiles()
      ]
      menu 'View', [
        menuItem 'Split Editor Pane Vertically', macKey: 'shift+ctrl+\\', pcKey: 'shift+ctrl+\\', callback: (controller) ->
          controller.viewSplitEditorPaneVertically()
        menuItem 'Split Editor Pane Horizontally', macKey: 'shift+ctrl+-', pcKey: 'shift+ctrl+-', callback: (controller) ->
          controller.viewSplitEditorPaneHorizontally()
        menuItem 'Close Editor Pane', macKey: "shift+ctrl+'", pcKey: "shift+ctrl+'", callback: (controller) ->
          controller.viewCloseEditorPane()
        separator
        menuItem 'Ruler', submenu: rulerMenu
        menuItem 'Wrap Long Lines',
          callback: (controller) ->
            controller.viewToggleWrapLines()
          checkedBinding: Em.Binding.oneWay('App.router.ideEditorController.wrapLines')
        separator
        menuItem 'Show Line Numbers',
          callback: (controller) ->
            controller.viewToggleLineNumbers()
          checkedBinding: Em.Binding.oneWay('App.router.ideEditorController.showLineNumbers')
        menuItem 'Highlight Line',
          callback: (controller) ->
            controller.viewToggleHighlightLine()
          checkedBinding: Em.Binding.oneWay('App.router.ideEditorController.highlightLine')
        menuItem 'Highlight Matching Brackets',
          callback: (controller) ->
            controller.viewToggleMatchBrackets()
          checkedBinding: Em.Binding.oneWay('App.router.ideEditorController.matchBrackets')
        menuItem 'Show Trailing Whitespace',
          callback: (controller) ->
            controller.viewToggleTrailingSpace()
          checkedBinding: Em.Binding.oneWay('App.router.ideEditorController.showTrailingSpace')
      ]
      menu 'Window', [
        menuItem 'Editor Theme', submenu: themesMenu
        separator
        menuItem 'Select Next Tab', macKey: 'shift+ctrl+]', pcKey: 'shift+ctrl+]', callback: (controller) ->
          controller.windowSelectNextTab()
        menuItem 'Select Previous Tab', macKey: 'shift+ctrl+[', pcKey: 'shift+ctrl+[', callback: (controller) ->
          controller.windowSelectPreviousTab()
        separator
        menuItem 'Focus Console', macKey: 'shift+ctrl+c', pcKey: 'ctrl+alt+c', callback: (controller) ->
          controller.windowFocusConsole()
        menuItem 'Minimize Console',
          hideBinding: Em.Binding.oneWay('App.consoleMinimized')
          callback: (controller) ->
            controller.windowMinimizeConsole()
        menuItem 'Restore Console',
          hide: (->
            !App.get('consoleMinimized')
          ).property('App.consoleMinimized')
          callback: (controller) ->
            controller.windowRestoreConsole()
        menuItem 'Toggle Full Screen Console', macKey: 'shift+ctrl+f', pcKey: 'ctrl+alt+f', callback: (controller) ->
          controller.windowToggleFullScreenConsole()
        separator
        menuItem 'Hide File Tree',
          hideBinding: Em.Binding.oneWay('App.fileTreeHidden')
          callback: (controller) ->
            controller.windowHideFileTree()
        menuItem 'Show File Tree',
          hide: (->
            !App.get('fileTreeHidden')
          ).property('App.fileTreeHidden')
          callback: (controller) ->
            controller.windowShowFileTree()
      ]
      menu 'Preview', @get('previewItems')
      menu 'Help', @get('helpMenuItems')
    ]
  ).property('previewItems', 'helpMenuItems')

  currentYear: (->
    new Date().getFullYear()
  ).property()

  actions:
    closeAboutDialog: ->
      ui.hideModal(@$('#about-nitrous'))
