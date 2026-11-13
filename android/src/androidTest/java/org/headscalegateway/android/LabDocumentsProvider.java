package org.headscalegateway.android;

import android.content.*;
import android.content.pm.PackageManager;
import android.database.*;
import android.net.Uri;
import android.os.*;
import android.provider.OpenableColumns;
import java.io.*;

/** Documentos de laboratorio: solo existe en el paquete de instrumentación. */
public final class LabDocumentsProvider extends ContentProvider {
    static final String AUTHORITY="org.headscalegateway.android.pruebas.documentos";
    static Uri uri(String name) {return Uri.parse("content://"+AUTHORITY+"/"+name);}
    @Override public boolean onCreate(){return true;}
    private File file(Uri uri) throws FileNotFoundException {
        if(getContext().getPackageManager().checkSignatures(Binder.getCallingUid(),android.os.Process.myUid())!=PackageManager.SIGNATURE_MATCH)throw new SecurityException("Documento reservado al laboratorio");
        String name=uri.getLastPathSegment();
        if(name==null || !name.matches("[A-Za-z0-9][A-Za-z0-9._-]{0,179}"))throw new FileNotFoundException("Nombre de prueba inválido");
        File root=new File(getContext().getCacheDir(),"documentos");root.mkdirs();return new File(root,name);
    }
    @Override public ParcelFileDescriptor openFile(Uri uri,String mode) throws FileNotFoundException {return ParcelFileDescriptor.open(file(uri),ParcelFileDescriptor.parseMode(mode));}
    @Override public Cursor query(Uri uri,String[] projection,String selection,String[] args,String order) {
        try {File f=file(uri);String[] columns=projection==null?new String[]{OpenableColumns.DISPLAY_NAME,OpenableColumns.SIZE}:projection;MatrixCursor c=new MatrixCursor(columns);Object[] row=new Object[columns.length];for(int i=0;i<row.length;i++)row[i]=columns[i].equals(OpenableColumns.DISPLAY_NAME)?f.getName():columns[i].equals(OpenableColumns.SIZE)?f.length():null;c.addRow(row);return c;}
        catch(FileNotFoundException e){return null;}
    }
    @Override public String getType(Uri uri){return "application/octet-stream";}
    @Override public Uri insert(Uri uri,ContentValues values){throw new UnsupportedOperationException();}
    @Override public int delete(Uri uri,String s,String[] a){try{return file(uri).delete()?1:0;}catch(Exception e){return 0;}}
    @Override public int update(Uri uri,ContentValues v,String s,String[] a){throw new UnsupportedOperationException();}
}
