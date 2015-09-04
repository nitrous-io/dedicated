App.IdeSidebarActiveFilesView = Em.View.extend
  templateName: 'ide/sidebar-active-files'
  elementId: 'ide-sidebar-active-files-view'

  activeFilesMonitorBinding: Em.Binding.oneWay 'controller.activeFilesMonitor'

  files: (->
    f = @get 'activeFilesMonitor.sortedFiles'
    console.log 'Sorted files: ', f
    f
  ).property 'activeFilesMonitor.sortedFiles'

