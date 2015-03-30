namespace :build do
  desc 'generate views'
  task generate_views: :environment do
    # map of desired html output path to render options
    locations = {
      '/public/app/index.html'        => {layout: 'layouts/web_app', template: 'web_app/index'},
      '/public/ide/index.html'        => {layout: 'layouts/web_app', template: 'web_app/ide'},
      '/public/index.html'            => {layout: 'layouts/pages', template: 'pages/home'},
    }

    %w(pricing features company plan_changes).each do |page|
      locations["/public/#{page}/index.html"] = { layout: 'layouts/pages', template: "pages/#{page}"}
    end

    %w(ruby-on-rails django nodejs meteor php html5 ruby python golang java wordpress iojs laravel ubuntu).each do |stack|
      locations["/public/stacks/#{stack}/index.html"] = { layout: 'layouts/pages', template: 'pages/stack', locals: { params: { stack: stack } } }
    end

    public_htmls = %w(403 404 422 500)

    build_dir = ENV['BUILD_DIR']
    raise 'Missing BUILD_DIR' if !build_dir

    locations.each do |path, opts|
      print "Rendering #{path}..."
      full_path = File.join(build_dir, path)
      FileUtils.mkdir_p(File.dirname(full_path))
      avb = ActionView::Base.new(Rails::Application::Configuration.new(Rails.root).paths['app/views'])
      File.write(full_path, avb.render(opts))
      puts "done"
    end

    public_htmls.each do |name|
      path = File.join('public', "#{name}.html")
      print "Copying #{path}..."
      FileUtils.cp File.join(Rails.root, path), File.join(build_dir, path)
      puts "done"
    end
  end
end
