class WebAppController < ApplicationController
  def index
  end

  def ide
  end

  def token_login
    render layout: 'bare'
  end

  def consent
    render layout: 'bare'
  end
end
