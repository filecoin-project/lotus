# Storing Data

Start by creating a file, in this example we will use the command line to create `hello.txt`.

```sh
$ echo "Hi my name is $USER" > hello.txt
```

Afterwards you can import the file into lotus & get a **Data CID** as output.

```sh
$ lotus client import ./hello.txt
<Data CID>
```

To see a list of files by `CID`, `name`, `size`, `status`.

```sh
$ lotus client local
```
