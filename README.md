# SpongeDownloads
SpongeDownloads is the download indexer for the Sponge project. It accepts uploaded builds as a standard Maven
repository, indexes them and finally uploads it to the real Maven repository. It provides a REST API that is primarily
used by [Sponge's download page](https://www.spongepowered.org/downloads), however it can be also used by other projects
(e.g. hosting providers) to provide automated installation of Sponge builds.

# API documentation
API documentation is available on [Apiary](https://dl-api.spongepowered.org/v1/).

## Running
SpongeDownloads uses the following environment variables:

- **Optional:** `PORT`: The port the application will listen on
- **Optional:** `MACARON_ENV=production` to minify responses
- **Optional:** `MODULES`: Comma separated list of modules to enable. By default, all modules are enabled.
  Available modules:

  - `indexer`: Maven repository proxy that indexes the uploaded artifacts
  - `api`: REST API implementation
  - `promote`: Promotion API endpoint for Jenkins (TBD)

- `POSTGRES_URL`: URL to PostgreSQL database instance
  - `postgres://postgres@localhost/downloads?sslmode=disable`

- **Optional:** `REDIRECT_ROOT` to redirect all requests to `/` to another URL
  - `https://www.spongepowered.org/#downloads`

- **Indexer:**
  - `UPLOAD_AUTH`: Username/password for authentication to upload artifacts
    - `user:password`
  - `GIT_STORAGE_DIR`: Directory to clone the Git repositories to, will be created automatically

- **Uploader:**
  - `UPLOAD_URL`: URL to Maven repository where the artifacts will be stored, e.g.:
    - `http://user:password@repo.example.com/maven`
    - `ftp://user:password@ftp.repo.example.com`
    - `file:///var/www/repo`
    - `null://` - Writes all uploaded files to `/dev/null`.

- **API:**
  - `REPO_URL`: URL to Maven repo, used for generating download URLs
    - `http://repo.example.com/maven`

- **Cache:** (Optional)
  - `CACHE`: Configure an additional reverse proxy to be used for additional caching. The application will
    automatically handle purging the caches when a new download is added. Supported formats:
    - `fastly:API_KEY/SERVICE_ID` with an optional `;healthcheck` option to hide Fastly health checks from the logs
