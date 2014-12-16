App.IdeSearchView = Em.View.extend
  templateName: 'ide/search'
  classNames: ['ide-search-view']

  document: null

  didClickResult: (filename, line) ->
    # filename will be relative to home directory
    path = App.Path.create absolutePath: filename
    if sidebar = App.get('router.ideSidebarController')
      sidebar.openPath path,
        done: (document) ->
          Em.run.next ->
            document.set('gotoLine', line.lineNumber)
        source: 'search'
