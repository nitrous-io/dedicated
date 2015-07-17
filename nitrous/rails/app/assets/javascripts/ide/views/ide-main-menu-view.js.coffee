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
  { name: 'solarized dark', label: 'Solarized Dark' }
  { name: 'vibrant-ink', label: 'Vibrant Ink' }
  { name: 'monokai-dark-soda', label: 'Monokai Dark Soda'}
  { name: 'base16-dark', label: 'Base16 Dark' }
  { name: 'base16-eighties-dark', label: 'Base16 Eighties Dark' }
  { name: 'base16-ocean-dark', label: 'Base16 Ocean Dark' }
  { name: '---' }
  { name: 'solarized light', label: 'Solarized Light' }
  { name: 'github', label: 'GitHub' }
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
        url: 'https://community.nitrous.io/docs'
        target: '_blank'
      menuItem 'Web IDE Overview',
        url: 'http://docs.nitrous.io/v1.0/docs/web-ide-overview'
        target: '_blank'
      menuItem 'Nitrous Account Questions',
        url: 'https://community.nitrous.io/docs/billing-pricing-faq'
        target: '_blank'
      separator
      menuItem 'Blog',
        url: 'https://community.nitrous.io/posts'
        target: '_blank'
      menuItem 'Updates',
        url: 'https://community.nitrous.io/posts/tagged/changelog'
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
        menuItem 'New',
          macKey: 'shift+ctrl+n', pcKey: 'ctrl+alt+n'
          chromeMacKey: ['command+n', 'ctrl+n'], chromePcKey: 'ctrl+n'
          callback: (controller) ->
            controller.fileNew()
        menuItem 'Save',
          macKey: ['command+s', 'ctrl+s', 'shift+ctrl+s'], pcKey: ['ctrl+s', 'ctrl+alt+s']
          chromeMacKey: ['command+s', 'ctrl+s'], chromePcKey: 'ctrl+s'
          disabled: (->
            App.get('router.ideEditorController.hasNoTab') || App.get('router.ideController.opsSocketNotReady')
          ).property 'App.router.ideEditorController.hasNoTab', 'App.router.ideController.opsSocketNotReady'
          callback: (controller) ->
            controller.fileSave()
        separator
        menuItem 'Refresh Tree',
          macKey: 'shift+ctrl+r', pcKey: 'ctrl+alt+r'
          chromeMacKey: ['shift+command+r', 'shift+ctrl+r'], chromePcKey: 'shift+ctrl+r'
          disabledBinding: Em.Binding.oneWay('App.router.ideSidebarController.opsSocketNotReady')
          callback: (controller) ->
            controller.fileRefreshTree()
        menuItem 'Go to File...',
          macKey: 'shift+ctrl+p', pcKey: 'shift+ctrl+p'
          chromeMacKey: ['command+p', 'ctrl+p'], chromePcKey: 'ctrl+p'
          disabledBinding: Em.Binding.oneWay('App.router.ideController.opsSocketNotReady')
          callback: (controller) ->
            controller.fileGoToFile()
        separator
        menuItem 'Close',
          macKey: ['shift+ctrl+w', 'ctrl+w'], pcKey: 'ctrl+alt+w'
          chromeMacKey: ['command+w', 'ctrl+w'], chromePcKey: 'ctrl+w'
          callback: (controller) ->
            controller.fileClose()
      ]
      menu 'Edit', [
        menuItem 'Go to Line...',
          macKey: ['command+l', 'ctrl+l'], pcKey: 'ctrl+l'
          chromeMacKey: ['command+l', 'ctrl+l'], chromePcKey: 'ctrl+l'
          disabledBinding: Em.Binding.oneWay('App.router.ideEditorController.hasNoTab')
          callback: (controller) ->
            controller.editGoToLine()
        separator
        menuItem 'Fold',
          macKey: ['option+command+[', 'ctrl+shift+['], pcKey: 'ctrl+shift+['
          chromeMacKey: ['option+command+[', 'ctrl+shift+['], chromePcKey: 'ctrl+shift+['
          disabledBinding: Em.Binding.oneWay('App.router.ideEditorController.isActiveTabNotAnEditor')
          callback: (controller) ->
            controller.editFoldCode()
        menuItem 'Unfold',
          macKey: ['option+command+]', 'ctrl+shift+]'], pcKey: 'ctrl+shift+]'
          chromeMacKey: ['option+command+]', 'ctrl+shift+]'], chromePcKey: 'ctrl+shift+]'
          disabledBinding: Em.Binding.oneWay('App.router.ideEditorController.isActiveTabNotAnEditor')
          callback: (controller) ->
            controller.editUnfoldCode()
        separator
        menuItem 'Toggle Comment',
          macKey: ['command+/', 'ctrl+/'], pcKey: 'ctrl+/'
          chromeMacKey: ['command+/', 'ctrl+/'], chromePcKey: 'ctrl+/'
          disabledBinding: Em.Binding.oneWay('App.router.ideEditorController.isActiveTabNotAnEditor')
          callback: (controller) ->
            controller.editToggleComment()
        menuItem 'Split Selection into Lines',
          macKey: 'shift+ctrl+l', pcKey: 'ctrl+shift+l'
          chromeMacKey: ['shift+command+l', 'ctrl+shift+l'], chromePcKey: 'ctrl+shift+l'
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
          macKey: 'command+f', pcKey: 'ctrl+f'
          chromeMacKey: ['command+f', 'ctrl+f'], chromePcKey: 'ctrl+f'
          disabledBinding: Em.Binding.oneWay('App.router.ideEditorController.isActiveTabNotAnEditor')
          callback: (controller) ->
            controller.findFind()
        separator
        menuItem 'Find Next',
          macKey: 'command+g', pcKey: 'ctrl+g'
          chromeMacKey: ['command+g', 'ctrl+g'], chromePcKey: ['ctrl+g', 'f3']
          disabledBinding: Em.Binding.oneWay('App.router.ideEditorController.isActiveTabNotAnEditor')
          callback: (controller) ->
            controller.findFindNext()
        menuItem 'Find Previous',
          macKey: ['shift+command+g'], pcKey: ['shift+ctrl+g', 'shift+f3']
          chromeMacKey: ['shift+command+g'], chromePcKey: ['shift+ctrl+g', 'shift+f3']
          disabledBinding: Em.Binding.oneWay('App.router.ideEditorController.isActiveTabNotAnEditor')
          callback: (controller) ->
            controller.findFindPrevious()
        menuItem 'Select Next',
          macKey: 'ctrl+d', pcKey: 'ctrl+d'
          chromeMacKey: ['command+d', 'ctrl+d'], chromePcKey: 'ctrl+d'
          disabledBinding: Em.Binding.oneWay('App.router.ideEditorController.isActiveTabNotAnEditor')
          callback: (controller) ->
            controller.findSelectNext()
        separator
        menuItem 'Replace',
          macKey: 'option+command+f', pcKey: 'shift+ctrl+h'
          chromeMacKey: ['option+command+f', 'ctrl+h'], chromePcKey: 'ctrl+h'
          disabledBinding: Em.Binding.oneWay('App.router.ideEditorController.isActiveTabNotAnEditor')
          callback: (controller) ->
            controller.findReplace()
        separator
        menuItem 'Find in Files...',
          macKey: 'shift+command+f', pcKey: 'ctrl+shift+f'
          chromeMacKey: 'shift+command+f', chromePcKey: 'ctrl+shift+f'
          disabledBinding: Em.Binding.oneWay('App.router.ideController.opsSocketNotReady')
          disabled: true
          callback: (controller) ->
            controller.findFindInFiles()
      ]
      menu 'View', [
        menuItem 'Split Editor Pane Vertically',
          macKey: 'shift+ctrl+\\', pcKey: 'ctrl+shift+\\'
          chromeMacKey: 'shift+ctrl+\\', chromePcKey: 'ctrl+shift+\\'
          callback: (controller) ->
            controller.viewSplitEditorPaneVertically()
        menuItem 'Split Editor Pane Horizontally',
          macKey: 'shift+ctrl+-', pcKey: 'ctrl+shift+-'
          chromeMacKey: 'shift+ctrl+-', chromePcKey: 'ctrl+shift+-'
          callback: (controller) ->
            controller.viewSplitEditorPaneHorizontally()
        menuItem 'Close Editor Pane',
          macKey: "shift+ctrl+'", pcKey: "shift+ctrl+'"
          chromeMacKey: "command+'", chromePcKey: "ctrl+'"
          callback: (controller) ->
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
        menuItem 'Select Next Tab',
          macKey: 'shift+ctrl+]', pcKey: 'ctrl+shift+]'
          chromeMacKey: ['ctrl+tab', 'shift+command+]', 'shift+ctrl+]'], chromePcKey: ['ctrl+tab', 'ctrl+shift+]']
          callback: (controller) ->
            controller.windowSelectNextTab()
        menuItem 'Select Previous Tab',
          macKey: 'shift+ctrl+[', pcKey: 'shift+ctrl+['
          chromeMacKey: ['shift+ctrl+tab', 'shift+command+[', 'shift+ctrl+['], chromePcKey: ['ctrl+shift+tab', 'ctrl+shift+[']
          callback: (controller) ->
            controller.windowSelectPreviousTab()
        separator
        menuItem 'Focus Console',
          macKey: 'shift+ctrl+c', pcKey: 'ctrl+alt+c'
          chromeMacKey: ['shift+command+c', 'shift+ctrl+c'], chromePcKey: 'ctrl+shift+c'
          callback: (controller) ->
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
        menuItem 'Toggle Full Screen Console',
          macKey: 'shift+ctrl+f', pcKey: 'ctrl+alt+f'
          chromeMacKey: ['shift+command+f', 'shift+ctrl+f'], chromePcKey: 'ctrl+shift+f'
          callback: (controller) ->
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
