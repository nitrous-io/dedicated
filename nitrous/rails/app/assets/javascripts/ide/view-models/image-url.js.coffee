#= require ./tab-content

App.ImageURL = App.TabContent.extend
  isImageURLTab: true

  title: (->
    @get('path.name')
  ).property 'path.name'
