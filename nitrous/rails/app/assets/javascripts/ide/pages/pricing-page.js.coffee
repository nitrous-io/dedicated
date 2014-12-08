window.App ||= {}

class App.VerticalSlider
  constructor: (@$el, @min, @max, @step) ->
    @$body = $('body')
    @$el.on 'mousedown.slider', (ev) => this.mouseDown(ev)
    @value = @min

  onChange: (value) ->

  mouseDown: (ev) ->
    this.mouseMove(ev)

    @$body.on 'mousemove.slider', (event) =>
      this.mouseMove event

    eventsOff = =>
      @$body.off 'mousemove.slider'
      @$body.off 'mouseup.slider'
      @$body.off 'mouseleave.slider'

    @$body.on 'mouseleave.slider', eventsOff
    @$body.on 'mouseup.slider', eventsOff

  mouseMove: (ev) ->
    ev.preventDefault()

    @$slider ?= @$el.find('.slider')
    @$track ?= @$el.find('.track')
    @$valueSpan ?= @$el.find('.value span')

    @trackHeight ?= @$track.height() - 7
    @offsetTop ?= @$track.offset().top

    rawValue = @trackHeight - (ev.pageY - @offsetTop)

    @value = @min + ((rawValue / @trackHeight) * (@max - @min))
    @value = Math.round(@value / @step) * @step
    @value = Math.max(Math.min(@value, @max), @min)

    @$slider.css 'bottom', Math.floor(((@value - @min) / (@max - @min)) * @trackHeight)
    @$valueSpan.text(@value)
    this.onChange(@value)

class App.PricingPage
  constructor: (@n2o128mbMemory, @n2o250mbStorage, @freeN2O, @n2oPriceCents) ->
    @boxSlider     = new App.VerticalSlider $('#box-slider'), 1, 10, 1
    @memorySlider  = new App.VerticalSlider $('#memory-slider'), 384, 2048, 128
    @storageSlider = new App.VerticalSlider $('#storage-slider'), 750, 20000, 250

    @boxCount = 1
    @memory = 384
    @storage = 750

    @n2o128mbMemory ?= 40
    @n2o250mbStorage ?= 10
    @freeN2O ?= 155
    @n2oPriceCents ?= 7

    this.updateValues()

    @boxSlider.onChange = (value) =>
      @boxCount = value
      this.updateValues()

    @memorySlider.onChange = (value) =>
      @memory = value
      this.updateValues()

    @storageSlider.onChange = (value) =>
      @storage = value
      this.updateValues()

  updateValues: ->
    @$totalN2O ?= $('#total-n2o')
    @$totalPrice ?= $('#total-price')

    totalN2O = Math.ceil((@memory / 128 * @n2o128mbMemory + @storage / 250 * @n2o250mbStorage) * @boxCount)
    totalPrice = Math.max((totalN2O - @freeN2O) * @n2oPriceCents / 100, 0)

    @$totalN2O.text(totalN2O)
    @$totalPrice.text(totalPrice.toFixed 2)
