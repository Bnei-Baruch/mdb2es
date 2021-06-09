# Backend for new archive site

## Overview

Backend for new archive site including, ETLs from BB Metadata DB to Elasticsearch.

## Commands
The archive-backend is meant to be executed as command line.
Type `archive-backend <command> -h` to see how to use each command.


```Shell
archive-backend server
```

Execute the backend api server for the new archive site.


```Shell
archive-backend version
```

Print the version of archive-backend


## Configuration

The default config file is `config.toml` in your current work directory.

See `config.sample.toml` for a sample config file.


## Release and Deployment

Once development is done, all tests are green, we want to go live.
All we have to do is simply execute `misc/release.sh`.

To add a pre-release tag, add the relevant environment variable. For example,

```Shell
PRE_RELEASE=rc.1 misc/release.sh
```


## MDB models

When MDB schema is changed we need to update the `mdb` package. Run this script:

```Shell
misc/update_mdb_models.sh
```

## Elasticsearch related stuff

(See the next section below for the instructions on installing Elasticsearch for Windows)

http://mrzard.github.io/blog/2015/03/25/elasticsearch-enable-mlockall-in-centos-7/
### Build index
There are two more dependencies required to build index:
1) Open Office (soffice binary) - to convert all doc to docx.
2) python-docx pyton library - to get text from docx
  - pip install python-docx

## Elasticsearch installation for Windows

1. Disable Go Modules.

    * Type from within the repository directory:

    ```Shell
    go env -w  GO111MODULE=off
    ```

    * For disabling go modules in VS Code extension see https://dev.to/codeboten/disabling-go-modules-in-visual-studio-code-31mp

2. Download and install the Java Virtual Machine for Windows from
http://www.oracle.com/technetwork/java/javase/downloads/jre8-downloads-2133155.html

![alt text](https://image.prntscr.com/image/PzmaOTOMQX2Bds_Dv_cXSA.png)

3. Download Elasticsearch 6.8.2 from
https://artifacts.elastic.co/downloads/elasticsearch/elasticsearch-6.8.2.zip

    Extract it to C:\elasticsearch-6.8.2

    If Elasticsearch keep crashing, consider adding these lines to **jvm.options** file in C:\elasticsearch-6.8.2\config

    * -Xms2g
    * -Xmx2g
        
    Xms represents the initial size of total heap space.
    Xmx represents the maximum size of total heap space.

4. Install the Hebrew dictionaries:

    * Download: he_IL.aff, he_IL.dic, settings.yml files from https://github.com/elastic/hunspell/tree/master/dicts/he_IL
    * Put these files under C:\elasticsearch-6.8.2\config\hunspell\he_IL
    * Additional dictionary terms are managed in .delta.dic files and located in search/hunspell directory of the repository. Copy these files into corresponding folders inside elasticsearch-6.8.2/config/hunspell/
    * Note that on Linux environment the supported format for Hebrew dictionary files is **ISO 8859-8**. 

5. Download and install Python - **version 2.7.x**
https://www.python.org/downloads/


6. Install python-docx (to get text from docx):

    * in CMD go to python directory

    ```Shell
    cd C:\Python27
    ```

    * and type

    ```Shell
    python -m pip install python-docx
    ```

7. Download and install LibreOffice (not OpenOffice!)

    https://www.libreoffice.org/donate/dl/win-x86_64/5.4.5/en-US/LibreOffice_5.4.5_Win_x64.msi

    Update 'soffice-bin' value with soffice.exe full path in config.toml, [elasticsearch] section:
    "C://Program Files//LibreOffice 5//program//soffice.exe"

8. Copy to config.toml the required commented-out lines from config.sample.toml that are related to Windows.

9. Updating assets:

    In order to make correct data indexing you should update the ES mapping configuration files (JSON files in /data/es/mappings):

    1. Exec. \es\mappings\make.py with python from the *root path* of the project. For example:
        ```Shell
        C:\Users\[USER]\go\src\github.com\Bnei-Baruch\archive-backend>python C:\Users\[USER]\go\src\github.com\Bnei-Baruch\archive-backend\es\mappings\make.py
        ```
    2. Repeat any time make.py is changed and executed.

## License

MIT
