# SpongeDownloads

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

- **Optional**: `API_AUTH`: Username/password for authentication to upload artifacts
  - `user:password`

- **Indexer**:
  - `UPLOAD_URL`: URL to Maven repository where the artifacts will be stored, e.g.:
    - `http://user:password@repo.example.com/maven`
    - `ftp://user:password@ftp.repo.example.com`
  - `GIT_STORAGE_DIR`: Directory to clone the Git repositories to, will be created automatically

- **API**:
  - `API_URL`: URL to Maven repo, used for generating download URLs
    - `http://repo.example.com/maven`
