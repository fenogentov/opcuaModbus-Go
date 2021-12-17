package notes

// // запись из файла в файл

//     b, err := os.ReadFile("input.txt")
//     if err != nil {
//         log.Fatal(err)
//     }

//     // `b` contains everything your file does
//     // This writes it to the Standard Out
//     os.Stdout.Write(b)

//     // You can also write it to a file as a whole
//     err = os.WriteFile("destination.txt", b, 0644)
//     if err != nil {
//         log.Fatal(err)
//     }

//     // read the whole file at once
//     b, err := ioutil.ReadFile("input.txt")
//     if err != nil {
//         panic(err)
//     }

//     // write the whole body at once
//     err = ioutil.WriteFile("output.txt", b, 0644)
//     if err != nil {
//         panic(err)
//     }

//     // open files r and w
//     r, err := os.Open("input.txt")
//     if err != nil {
//         panic(err)
//     }
//     defer r.Close()

//     w, err := os.Create("output.txt")
//     if err != nil {
//         panic(err)
//     }
//     defer w.Close()

//     // do the actual work
//     n, err := io.Copy(w, r)
//     if err != nil {
//         panic(err)
//     }
//     log.Printf("Copied %v bytes\n", n)

// использовать w.Sync()после io.Copy(w, r)
